// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package inmemory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/informer"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory/filter"
	"github.com/telekom/controlplane/common-server/pkg/store/inmemory/patch"
	"github.com/tidwall/gjson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ store.ObjectStore[store.Object] = &InmemoryObjectStore[store.Object]{}
var _ informer.EventHandler = &InmemoryObjectStore[store.Object]{}

type StoreOpts struct {
	Client       dynamic.Interface
	GVR          schema.GroupVersionResource
	GVK          schema.GroupVersionKind
	AllowedSorts []string

	Database DatabaseOpts
	Informer InformerOpts

	// DisableRetryOnConflict disables retrying on conflict errors during updates.
	// By default, retries are enabled.
	DisableRetryOnConflict bool
}

type InformerOpts struct {
	DisableCache bool
}

type DatabaseOpts struct {
	// Filepath will store the badger database on disk at the given filepath.
	// A non-empty Filepath selects the reduced-memory Badger profile;
	// an empty Filepath keeps the database in memory with the default profile.
	Filepath string
}

type InmemoryObjectStore[T store.Object] struct {
	ctx             context.Context
	log             logr.Logger
	gvr             schema.GroupVersionResource
	gvk             schema.GroupVersionKind
	k8sClient       dynamic.NamespaceableResourceInterface
	informer        informer.Informer
	db              *badger.DB
	allowedSorts    []string
	sortValueCache  sync.Map
	retryOnConflict bool

	// mutationMu serializes OnUpdate and OnDelete so the database,
	// sortValueCache and versionCache stay consistent.
	mutationMu sync.Mutex
	// versionCache maps keys to the Kubernetes version of the stored object.
	// Only objects with a non-empty resourceVersion are recorded.
	// Guarded by mutationMu.
	versionCache map[string]cachedObjectVersion
}

// cachedObjectVersion identifies a stored object revision. The
// resourceVersion is compared for equality only; it is opaque.
type cachedObjectVersion struct {
	uid             types.UID
	resourceVersion string
}

func objectVersion(obj *unstructured.Unstructured) cachedObjectVersion {
	return cachedObjectVersion{uid: obj.GetUID(), resourceVersion: obj.GetResourceVersion()}
}

// isStoredLocked reports whether key already holds version.
// Versionless objects are never considered stored. Requires mutationMu.
func (s *InmemoryObjectStore[T]) isStoredLocked(key string, version cachedObjectVersion) bool {
	if version.resourceVersion == "" {
		return false
	}
	prev, ok := s.versionCache[key]
	return ok && prev == version
}

// isStored is isStoredLocked acquiring mutationMu.
func (s *InmemoryObjectStore[T]) isStored(key string, version cachedObjectVersion) bool {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.isStoredLocked(key, version)
}

const (
	diskValueThreshold     = 4 << 10 // 4 KiB
	inMemoryValueThreshold = 1 << 20 // 1 MiB
)

// valueThreshold returns the Badger value threshold for the storage mode.
// On disk, values of at least this size are stored in the value log.
// In memory, values must be strictly smaller than this size.
func valueThreshold(inMemory bool) int64 {
	if inMemory {
		return inMemoryValueThreshold
	}
	return diskValueThreshold
}

// newBadgerOpts selects the Badger profile from the storage mode:
// on disk the reduced-memory profile, in memory the default profile.
func newBadgerOpts(path string) badger.Options {
	if path == "" {
		return newBadgerOptsInMemory()
	}
	return newBadgerOptsDisk(path)
}

func newBadgerOptsDisk(path string) badger.Options {
	return badger.
		DefaultOptions(path).
		WithInMemory(false).
		WithMetricsEnabled(false).
		WithIndexCacheSize(0).
		WithNumMemtables(2).
		WithMemTableSize(32 << 20).     // 32 MiB
		WithValueLogFileSize(64 << 20). // 64 MiB
		WithBlockCacheSize(32 << 20).   // 32 MiB
		WithBlockSize(4 << 10).         // 4 KiB
		WithValueThreshold(valueThreshold(false)).
		WithBloomFalsePositive(0.01).
		WithCompression(options.Snappy)
}

func newBadgerOptsInMemory() badger.Options {
	return badger.
		DefaultOptions("").
		WithInMemory(true).
		WithMetricsEnabled(false).
		WithIndexCacheSize(0).
		WithNumMemtables(4).
		WithMemTableSize(16 << 20).      // 16 MiB
		WithValueLogFileSize(512 << 20). // 512 MiB
		WithBlockCacheSize(64 << 20).    // 64 MiB
		WithBlockSize(4 << 10).          // 4 KiB
		WithValueThreshold(valueThreshold(true)).
		WithBloomFalsePositive(0.01).
		WithCompression(options.Snappy)
}

// isSafeDbName reports whether name is a single local path element,
// so joining it to the root yields a direct child of the root.
func isSafeDbName(name string) bool {
	return filepath.IsLocal(name) &&
		filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/`+string(os.PathSeparator))
}

func newDbOrDie(storeOpts StoreOpts, log logr.Logger) *badger.DB {
	useFilesystem := storeOpts.Database.Filepath != ""
	path := ""
	if useFilesystem {
		dbName := fmt.Sprintf("db-%s-%s-%s",
			strings.ToLower(storeOpts.GVR.Group),
			strings.ToLower(storeOpts.GVR.Version),
			strings.ToLower(storeOpts.GVR.Resource),
		)
		if !isSafeDbName(dbName) {
			panic(errors.Errorf("invalid badger DB directory name %q derived from GVR %s", dbName, storeOpts.GVR.String()))
		}
		path = filepath.Join(storeOpts.Database.Filepath, dbName)

		// The informer repopulates the store on startup; stale data from a
		// previous run must not survive. Only the per-store directory is removed.
		log.Info("resetting badger DB directory", "path", path)
		if err := os.RemoveAll(path); err != nil {
			panic(errors.Wrapf(err, "failed to reset badger DB directory %q", path))
		}
	}

	log.Info("initializing badger DB",
		"inMemory", !useFilesystem,
		"path", path,
		"reduceMemory", useFilesystem,
	)

	opts := newBadgerOpts(path)

	opts.Logger = NewLoggerShim(log)
	db, err := badger.Open(opts)
	if err != nil {
		panic(errors.Wrap(err, "failed to create in-memory store"))
	}
	return db
}

func NewOrDie[T store.Object](ctx context.Context, storeOpts StoreOpts) *InmemoryObjectStore[T] {
	store := &InmemoryObjectStore[T]{
		ctx:             ctx,
		log:             logr.FromContextOrDiscard(ctx),
		gvr:             storeOpts.GVR,
		gvk:             storeOpts.GVK,
		k8sClient:       storeOpts.Client.Resource(storeOpts.GVR),
		retryOnConflict: !storeOpts.DisableRetryOnConflict,
	}
	var err error
	store.db = newDbOrDie(storeOpts, store.log)

	if storeOpts.Informer.DisableCache {
		store.log.Info("disabling informer cache")
		store.informer = informer.NewNoCache(ctx, store.gvr, storeOpts.Client, store)
	} else {
		store.informer = informer.New(ctx, store.gvr, storeOpts.Client, store)
	}

	if err = store.informer.Start(); err != nil {
		panic(errors.Wrap(err, "failed to start informer"))
	}

	startValueLogGC(ctx, store.db, store.log)

	return store
}

func (s *InmemoryObjectStore[T]) Info() (schema.GroupVersionResource, schema.GroupVersionKind) {
	return s.gvr, s.gvk
}

func (s *InmemoryObjectStore[T]) Ready() bool {
	return s.informer.Ready()
}

func (s *InmemoryObjectStore[T]) Get(ctx context.Context, namespace, name string) (result T, err error) {
	key := newKey(namespace, name)
	err = s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return problems.NotFound(key)
			}
			return errors.Wrapf(err, "failed to get item %s", key)
		}
		if item == nil {
			return problems.NotFound(key)
		}

		return item.Value(func(val []byte) error {
			return sonic.Unmarshal(val, &result)
		})
	})

	return result, err
}

func (s *InmemoryObjectStore[T]) List(ctx context.Context, listOpts store.ListOpts) (result *store.ListResponse[T], err error) {
	s.log.V(1).Info("list", "limit", listOpts.Limit, "cursor", listOpts.Cursor)

	hasFilters := len(listOpts.Filters) > 0

	result = &store.ListResponse[T]{
		Items: make([]T, listOpts.Limit),
	}

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(listOpts.Prefix)
	if !hasFilters {
		opts.PrefetchValues = true
	} else {
		opts.PrefetchValues = false
	}

	var filterFunc filter.FilterFunc

	if hasFilters {
		filterFunc = filter.NewFilterFuncs(listOpts.Filters)
	} else {
		filterFunc = filter.NopFilter
	}

	startCursor := []byte(listOpts.Cursor)
	prefix := opts.Prefix
	startKey := prefix
	if len(startCursor) > 0 {
		startKey = startCursor
	}

	limit := listOpts.Limit
	iterNum := 0

	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(startKey); it.ValidForPrefix(prefix); it.Next() {
			var err error
			var value []byte
			key := string(it.Item().Key())

			if iterNum >= limit {
				s.log.V(1).Info("limit reached", "limit", limit, "cursor", key)
				result.Links.Next = key
				return nil
			}

			if !hasFilters {
				value, err = it.Item().ValueCopy(nil)
				if err != nil {
					return err
				}

			} else {
				err = it.Item().Value(func(val []byte) error {
					if !filterFunc(val) {
						return nil
					}
					value = val
					return nil
				})
				if err != nil {
					return err
				}
			}

			if value != nil {
				err = sonic.Unmarshal(value, &result.Items[iterNum])
				if err != nil {
					return errors.Wrap(err, "invalid object")
				}
				if result.Links.Self == "" {
					result.Links.Self = key
				}
				iterNum++
			}
		}
		return nil
	})

	if iterNum < listOpts.Limit {
		result.Links.Next = ""
		// truncate the list
		result.Items = result.Items[:iterNum]
	}

	return result, err
}

func (s *InmemoryObjectStore[T]) Delete(ctx context.Context, namespace, name string) error {
	err := s.k8sClient.Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return mapErrorToProblem(err)
	}
	return nil
}

// CreateOrReplace creates or replaces the given object in the store.
// It will use the Kubernetes API to create or update the object and store
// the result in this store.
// It does not update the object in-place
// To get the latest version of the object, use Get after CreateOrReplace.
func (s *InmemoryObjectStore[T]) CreateOrReplace(ctx context.Context, in T) error {
	if in.GetName() == "" {
		return problems.ValidationError("metadata.name", "name is required")
	}
	if in.GetNamespace() == "" {
		return problems.ValidationError("metadata.namespace", "namespace is required")
	}

	obj, err := convertToUnstructured(in)
	if err != nil {
		return errors.Wrap(err, "failed to convert object")
	}

	currentObj, err := s.Get(ctx, obj.GetNamespace(), obj.GetName())
	if err != nil && !problems.IsNotFound(err) {
		return err
	}

	// Initial creation if object does not exist
	if problems.IsNotFound(err) {
		obj.GetObjectKind().SetGroupVersionKind(s.gvk)
		s.log.Info("creating object", "namespace", obj.GetNamespace(), "name", obj.GetName(), "gvk", obj.GetObjectKind().GroupVersionKind())

		obj, err = s.k8sClient.Namespace(obj.GetNamespace()).Create(ctx, obj, metav1.CreateOptions{
			FieldValidation: "Strict",
		})
		if err != nil {
			return errors.Wrapf(mapErrorToProblem(err), "failed to create object %v/%v", in.GetNamespace(), in.GetName())
		}
		return s.OnCreate(ctx, obj)
	}

	// Object exists, update it

	obj.SetResourceVersion(currentObj.GetResourceVersion())
	obj, err = s.k8sClient.Namespace(obj.GetNamespace()).Update(ctx, obj, metav1.UpdateOptions{
		FieldValidation: "Strict",
	})
	if err == nil {
		return s.OnUpdate(ctx, obj)
	}
	// if not retrying on conflict or error is not conflict, return error
	if !s.retryOnConflict || !apierrors.IsConflict(err) {
		return errors.Wrap(mapErrorToProblem(err), "failed to update object")
	}

	obj, err = convertToUnstructured(in)
	if err != nil {
		return errors.Wrap(err, "failed to convert object")
	}

	// try to resolve conflict by getting the latest version and retrying the update
	currentObjInCluster, err := s.k8sClient.Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(mapErrorToProblem(err), "failed to get object")
	}
	obj.SetResourceVersion(currentObjInCluster.GetResourceVersion())
	obj, err = s.k8sClient.Namespace(obj.GetNamespace()).Update(ctx, obj, metav1.UpdateOptions{
		FieldValidation: "Strict",
	})
	if err != nil {
		return errors.Wrap(mapErrorToProblem(err), "failed to update object")
	}

	return s.OnUpdate(ctx, obj)
}

func (s *InmemoryObjectStore[T]) Patch(ctx context.Context, namespace, name string, ops ...store.Patch) (obj T, err error) {

	if len(ops) == 0 {
		return obj, errors.New("no patch operations provided")
	}

	var value []byte
	patchFunc := patch.NewPatchFuncs(ops)

	err = s.db.View(func(txn *badger.Txn) error {
		key := newKey(namespace, name)
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}
		if item == nil {
			return nil // Not found
		}

		err = item.Value(func(val []byte) error {
			value = val
			return nil
		})

		if err != nil {
			return errors.Wrap(err, "failed to get value")
		}

		return nil
	})

	if err != nil {
		return obj, errors.Wrap(err, "failed to get value")
	}

	if value == nil {
		return obj, errors.New("object not found")
	}

	value, err = patchFunc(value)
	if err != nil {
		return obj, errors.Wrap(err, "failed to patch object")
	}

	err = sonic.Unmarshal(value, &obj)
	if err != nil {
		return obj, errors.Wrap(err, "failed to unmarshal patched object")
	}
	return obj, s.CreateOrReplace(ctx, obj)
}

func (s *InmemoryObjectStore[T]) OnCreate(ctx context.Context, obj *unstructured.Unstructured) error {
	return s.OnUpdate(ctx, obj)
}

func (s *InmemoryObjectStore[T]) OnUpdate(ctx context.Context, obj *unstructured.Unstructured) error {
	key := calculateKey(obj)
	// Kubernetes metadata is authoritative: an already stored UID and
	// resourceVersion is skipped before copying and serializing.
	if s.isStored(key, objectVersion(obj)) {
		return nil
	}

	obj = obj.DeepCopy()
	version := objectVersion(obj)
	informer.SanitizeObject(obj)

	data, err := sonic.Marshal(obj.Object)
	if err != nil {
		return errors.Wrap(err, "invalid object")
	}
	// Badger's exposed ValueThreshold is not the effective in-memory limit, and
	// values equal to the limit would be routed to the absent value log.
	if s.db.Opts().InMemory {
		limit := valueThreshold(true)
		if int64(len(data)) >= limit {
			return errors.Errorf("object %s is too large for in-memory store: %d bytes, limit is less than %d bytes", key, len(data), limit)
		}
	}

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	if s.isStoredLocked(key, version) {
		return nil
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
	if err != nil {
		return err
	}

	s.sortValueCache.Range(func(k, v any) bool {
		sp := k.(string)
		m := v.(*sync.Map)
		value := gjson.GetBytes(data, sp)
		if !value.Exists() {
			m.Delete(key)
			return true
		}
		m.Store(key, value.Value())
		s.log.V(1).Info("cached sort value", "key", key, "sortPath", sp)
		return true
	})

	if version.resourceVersion == "" {
		// A versionless write replaces any versioned revision; drop its
		// entry so a later event with the old version is written again.
		delete(s.versionCache, key)
		return nil
	}
	if s.versionCache == nil {
		s.versionCache = make(map[string]cachedObjectVersion)
	}
	s.versionCache[key] = version
	return nil
}

func (s *InmemoryObjectStore[T]) OnDelete(ctx context.Context, obj *unstructured.Unstructured) error {
	key := calculateKey(obj)

	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	err := s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
	if err != nil {
		return err
	}
	s.sortValueCache.Range(func(k, v any) bool {
		m := v.(*sync.Map)
		m.Delete(key)
		s.log.V(1).Info("deleted cached sort value", "key", key, "sortPath", k)
		return true
	})
	delete(s.versionCache, key)
	return nil
}

func mapErrorToProblem(err error) problems.Problem {
	if err == nil {
		return nil
	}
	apiStatus, ok := err.(apierrors.APIStatus)
	if !ok {
		return problems.NewProblemOfError(err)
	}
	status := apiStatus.Status()

	switch status.Code {
	case 404:
		return problems.NotFound(status.Kind)
	case 400, 422:
		if status.Details == nil {
			return problems.BadRequest(status.Message)
		}
		causes := status.Details.Causes
		if len(causes) == 0 {
			return problems.BadRequest(status.Message)
		}

		fields := make(map[string]string, len(causes))
		for _, cause := range causes {
			if _, ok := fields[cause.Field]; ok {
				fields[cause.Field] += ", " + cause.Message
				continue
			}
			fields[cause.Field] = cause.Message
		}
		return problems.ValidationErrors(fields)

	case 409:
		return problems.Conflict(status.Message)

	default:
		return problems.NewProblemOfError(err)
	}
}

func calculateKey(obj store.Object) string {
	return newKey(obj.GetNamespace(), obj.GetName())
}

func newKey(namespace, name string) string {
	return namespace + "/" + name + "/"
}

func convertToUnstructured(obj any) (*unstructured.Unstructured, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: u}, nil
}
