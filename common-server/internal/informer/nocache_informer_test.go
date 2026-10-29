// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package informer_test

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/informer"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	dynamic "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("NoCacheInformer", func() {

	var ctx context.Context
	var cancel context.CancelFunc
	gvr := schema.GroupVersionResource{
		Group:    "testgroup",
		Version:  "v1",
		Resource: "testobjects",
	}
	var eventHandler *mockEventHandler
	var mockClient *dynamic.FakeDynamicClient

	Context("Informer", Ordered, func() {

		BeforeEach(func() {
			ctx = logr.NewContext(context.Background(), GinkgoLogr)
			ctx, cancel = context.WithCancel(ctx)
			mockClient = dynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
				gvr: "TestObjectList",
			})
			eventHandler = &mockEventHandler{}

		})

		It("should initialize a new Informer instance", func() {
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
		})

		It("should start the Informer", func() {
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			Eventually(func(g Gomega) {
				g.Expect(inf.Ready()).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})

		It("should stop the informer gracefully", func() {
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			cancel()

		})

		It("should handle events", func() {
			resourceClient := mockClient.Resource(gvr).Namespace("default")
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())
			obj := NewUnstructured("test")

			obj, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("create"))
				g.Expect(eventHandler.Events()[0].Object).To(Equal(obj))
			}, timeout, interval).Should(Succeed())

			eventHandler.Reset()

			obj.SetLabels(map[string]string{"foo": "bar"})

			_, err = resourceClient.Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("update"))
			}, timeout, interval).Should(Succeed())

			eventHandler.Reset()

			err = resourceClient.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(eventHandler.Events()).To(HaveLen(1))
				g.Expect(eventHandler.Events()[0].Action).To(Equal("delete"))
			}, timeout, interval).Should(Succeed())

		})

		It("should handle errors", func() {
			eventHandler.NextError = errors.New("FAKE_ERROR")
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())

			resourceClient := mockClient.Resource(gvr).Namespace("default")
			obj := NewUnstructured("test")
			_, err = resourceClient.Create(ctx, obj, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

		})

		It("should reload on resource version expiration", func() {
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())

			// simulate resource version expiration by directly calling Reload
			err = inf.Reload()
			Expect(err).ToNot(HaveOccurred())

		})

		Context("recovery", func() {
			fastBackoff := wait.Backoff{Duration: 10 * time.Millisecond, Factor: 2, Jitter: 0.1, Steps: 3, Cap: 50 * time.Millisecond}

			start := func(options informer.NoCacheInformerOptions) *informer.NoCacheInformer {
				inf := informer.NewNoCacheWithOptions(ctx, gvr, mockClient, eventHandler, options)
				inf.SetTestTimings(fastBackoff, 100*time.Millisecond)
				Expect(inf.Start()).To(Succeed())
				DeferCleanup(inf.Stop)
				return inf
			}

			It("should retry the watch after a failed reload without waiting for the resync", func() {
				var watchCalls atomic.Int32
				first, second := watch.NewFake(), watch.NewFake()
				mockClient.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
					switch watchCalls.Add(1) {
					case 1:
						return true, first, nil
					case 2:
						return true, nil, errors.New("api server unavailable")
					default:
						return true, second, nil
					}
				})

				start(informer.DefaultNoCacheInformerOptions())
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(1))

				first.Stop()

				Eventually(watchCalls.Load, timeout, interval).Should(BeNumerically(">=", 3))
				second.Add(NewUnstructured("after-recovery"))
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "after-recovery")))
			})

			It("should emit DELETED on relist for objects that are no longer listed", func() {
				current, _ := fakeWatches(mockClient)

				resourceClient := mockClient.Resource(gvr).Namespace("default")
				_, err := resourceClient.Create(ctx, NewUnstructured("kept"), metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				_, err = resourceClient.Create(ctx, NewUnstructured("gone"), metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				start(informer.DefaultNoCacheInformerOptions())
				Eventually(eventHandler.Events, timeout, interval).Should(HaveLen(2))
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())
				eventHandler.Reset()

				// Deleted while the watch does not deliver it, e.g. during an API server outage.
				Expect(resourceClient.Delete(ctx, "gone", metav1.DeleteOptions{})).To(Succeed())
				// Resource version expired: forces a full relist.
				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})

				Eventually(func(g Gomega) {
					events := eventHandler.Events()
					g.Expect(events).To(ContainElement(HaveEvent("delete", "gone")))
					g.Expect(events).To(ContainElement(HaveEvent("create", "kept")))
					g.Expect(events).NotTo(ContainElement(HaveEvent("delete", "kept")))
				}, timeout, interval).Should(Succeed())
			})

			It("should retry a list whose second page failed and become ready", func() {
				fakeWatches(mockClient)
				var page2Failures atomic.Int32
				page2Failures.Store(1)
				pagedLists(mockClient, func(continueToken string) ([]string, string, error) {
					if continueToken == "" {
						return []string{"a"}, "page-2", nil
					}
					if page2Failures.Add(-1) >= 0 {
						return nil, "", errors.New("api server unavailable")
					}
					return []string{"b"}, "", nil
				})

				inf := start(informer.DefaultNoCacheInformerOptions())

				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElements(HaveEvent("create", "a"), HaveEvent("create", "b")))
			})

			It("should emit DELETED when a relist is retried after its second page failed", func() {
				current, _ := fakeWatches(mockClient)
				var relist atomic.Bool
				var page2Failures atomic.Int32
				page2Failures.Store(1)
				pagedLists(mockClient, func(continueToken string) ([]string, string, error) {
					switch {
					case !relist.Load():
						return []string{"a", "b"}, "", nil
					case continueToken == "":
						return []string{"a"}, "page-2", nil
					case page2Failures.Add(-1) >= 0:
						return nil, "", errors.New("api server unavailable")
					default:
						return nil, "", nil
					}
				})

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(eventHandler.Events, timeout, interval).Should(HaveLen(2))
				eventHandler.Reset()

				relist.Store(true)
				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})

				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("delete", "b")))
				Expect(eventHandler.Events()).NotTo(ContainElement(HaveEvent("delete", "a")))
			})

			It("should emit DELETED on relist for tracked objects that are no longer listed", func() {
				current, _ := fakeWatches(mockClient)
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"kept"}, "", nil
				})

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				// Written by the event handler itself, then deleted while the
				// watch does not deliver it.
				inf.Track(NewUnstructured("written"))
				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})

				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("delete", "written")))
				Expect(eventHandler.Events()).NotTo(ContainElement(HaveEvent("delete", "kept")))
			})

			deletes := func(name string) func() int {
				return func() int { return CountEvents(eventHandler.Events(), "delete", name) }
			}

			It("should emit DELETED again on relist after a watched delete failed", func() {
				current, _ := fakeWatches(mockClient)
				var names atomic.Pointer[[]string]
				names.Store(&[]string{"a"})
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return *names.Load(), "", nil
				})

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				names.Store(&[]string{})
				eventHandler.FailOnce("delete")
				current.Load().Delete(NewUnstructured("a"))
				Eventually(deletes("a"), timeout, interval).Should(Equal(1))

				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(deletes("a"), timeout, interval).Should(Equal(2))
			})

			It("should emit DELETED again on the next relist after a relisted delete failed", func() {
				current, watchCalls := fakeWatches(mockClient)
				var names atomic.Pointer[[]string]
				names.Store(&[]string{"a"})
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return *names.Load(), "", nil
				})

				start(informer.DefaultNoCacheInformerOptions())
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(1))

				names.Store(&[]string{})
				eventHandler.FailOnce("delete")
				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(deletes("a"), timeout, interval).Should(Equal(1))
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(2))

				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(deletes("a"), timeout, interval).Should(Equal(2))
			})

			It("should resend an object on relist after its update failed", func() {
				current, _ := fakeWatches(mockClient)
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a"}, "", nil
				})

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				eventHandler.FailOnce("update")
				current.Load().Modify(NewUnstructured("a"))
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("update", "a")))
				eventHandler.Reset()

				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "a")))
			})

			It("should keep tracking an object re-created while its delete was handled", func() {
				current, _ := fakeWatches(mockClient)
				var names atomic.Pointer[[]string]
				names.Store(&[]string{"a"})
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return *names.Load(), "", nil
				})

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				eventHandler.Block()
				DeferCleanup(eventHandler.Unblock)
				w := current.Load()
				w.Delete(NewUnstructured("a"))
				w.Add(NewUnstructured("a"))
				w.Add(NewUnstructured("sentinel")) // returns once the re-create was queued
				eventHandler.Unblock()
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "sentinel")))
				Eventually(func() int { return CountEvents(eventHandler.Events(), "create", "a") }, timeout, interval).Should(Equal(2))

				// Deleted again while the watch does not deliver it.
				names.Store(&[]string{"sentinel"})
				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(deletes("a"), timeout, interval).Should(Equal(2))
			})

			It("should list everything on resync", func() {
				fakeWatches(mockClient)
				listCalls := pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a"}, "", nil
				})

				options := informer.DefaultNoCacheInformerOptions()
				options.ResyncPeriod = 200 * time.Millisecond
				start(options)

				Eventually(listCalls.Load, timeout, 20*time.Millisecond).Should(BeNumerically(">=", 2))
			})

			It("should leave objects tracked during a list to the next relist", func() {
				current, watchCalls := fakeWatches(mockClient)
				var inf atomic.Pointer[informer.NoCacheInformer]
				var listCalls atomic.Int32
				pagedLists(mockClient, func(string) ([]string, string, error) {
					if listCalls.Add(1) == 2 {
						// Created after the list snapshot.
						inf.Load().Track(NewUnstructured("racing"))
					}
					return []string{"kept"}, "", nil
				})

				inf.Store(start(informer.DefaultNoCacheInformerOptions()))
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(1))

				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(2))
				Consistently(eventHandler.Events, 200*time.Millisecond, 20*time.Millisecond).ShouldNot(ContainElement(HaveEvent("delete", "racing")))

				current.Load().Error(&metav1.Status{Code: 410, Reason: metav1.StatusReasonExpired})
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("delete", "racing")))
			})

			It("should relist after a watch event was dropped and repair it", func() {
				current, _ := fakeWatches(mockClient)
				var names atomic.Pointer[[]string]
				names.Store(&[]string{"kept", "gone"})
				listCalls := pagedLists(mockClient, func(string) ([]string, string, error) {
					return *names.Load(), "", nil
				})

				options := informer.DefaultNoCacheInformerOptions()
				options.QueueSize = 1
				options.WorkerCount = 1
				inf := start(options)
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(eventHandler.Events, timeout, interval).Should(HaveLen(2))
				eventHandler.Reset()

				names.Store(&[]string{"kept"})
				eventHandler.Block()
				DeferCleanup(eventHandler.Unblock)
				w := current.Load()
				w.Modify(NewUnstructured("kept")) // taken by the blocked worker
				w.Modify(NewUnstructured("kept")) // fills the queue
				w.Delete(NewUnstructured("gone")) // dropped after the enqueue timeout

				Eventually(listCalls.Load, timeout, interval).Should(BeEquivalentTo(2))
				eventHandler.Unblock()

				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("delete", "gone")))
			})

			It("should handle the events of one object in order", func() {
				current, _ := fakeWatches(mockClient)
				options := informer.DefaultNoCacheInformerOptions()
				options.WorkerCount = 8
				inf := start(options)
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				w := current.Load()
				sent := make([]string, 0, 200)
				for n := range 200 {
					obj := NewUnstructured("same")
					obj.SetLabels(map[string]string{"seq": strconv.Itoa(n)})
					w.Modify(obj)
					sent = append(sent, strconv.Itoa(n))
				}

				Eventually(func(g Gomega) {
					events := eventHandler.Events()
					handled := make([]string, 0, len(events))
					for _, e := range events {
						handled = append(handled, e.Object.GetLabels()["seq"])
					}
					g.Expect(handled).To(Equal(sent))
				}, timeout, interval).Should(Succeed())
			})

			It("should restart the watch before an event that was not queued", func() {
				current, _ := fakeWatches(mockClient)
				watchRVs := watchResourceVersions(mockClient)
				pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a", "b"}, "", nil
				})

				options := informer.DefaultNoCacheInformerOptions()
				options.QueueSize = 1
				options.WorkerCount = 1
				inf := informer.NewNoCacheWithOptions(ctx, gvr, mockClient, eventHandler, options)
				inf.SetTestTimings(fastBackoff, time.Minute)
				Expect(inf.Start()).To(Succeed())
				DeferCleanup(inf.Stop)
				Eventually(eventHandler.Events, timeout, interval).Should(HaveLen(2))
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				eventHandler.Block()
				DeferCleanup(eventHandler.Unblock)
				w := current.Load()
				w.Modify(NewUnstructuredWithResourceVersion("a", "11")) // taken by the blocked worker
				w.Modify(NewUnstructuredWithResourceVersion("a", "12")) // fills the queue
				w.Modify(NewUnstructuredWithResourceVersion("b", "13")) // waits for room in the queue

				Expect(inf.Reload()).To(Succeed())

				Expect(watchRVs()).To(Equal([]string{"10", "12"}))
			})

			It("should stop the previous watcher on reload", func() {
				current, watchCalls := fakeWatches(mockClient)
				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(1))
				first := current.Load()

				Expect(inf.Reload()).To(Succeed())

				Expect(watchCalls.Load()).To(BeEquivalentTo(2))
				Expect(first.IsStopped()).To(BeTrue())
				Expect(current.Load()).NotTo(BeIdenticalTo(first))
			})

			It("should return from Stop while a list is blocked on a full queue", func() {
				listCalls := pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a", "b", "c", "d", "e"}, "", nil
				})
				eventHandler.Block()
				DeferCleanup(eventHandler.Unblock)

				options := informer.DefaultNoCacheInformerOptions()
				options.QueueSize = 1
				options.WorkerCount = 1
				inf := informer.NewNoCacheWithOptions(ctx, gvr, mockClient, eventHandler, options)
				inf.SetTestTimings(fastBackoff, 100*time.Millisecond)
				Expect(inf.Start()).To(Succeed())
				Eventually(listCalls.Load, timeout, interval).Should(BeEquivalentTo(1))
				Consistently(inf.Ready, 200*time.Millisecond, 20*time.Millisecond).Should(BeFalse())

				stopped := make(chan struct{})
				go func() {
					inf.Stop()
					close(stopped)
				}()

				Eventually(stopped, 2*time.Second, 20*time.Millisecond).Should(BeClosed())
			})

			It("should keep the resource version when the watch fails with a non-expiry error", func() {
				var watchCalls atomic.Int32
				mockClient.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
					if watchCalls.Add(1) <= 3 {
						return true, nil, apierrors.NewForbidden(gvr.GroupResource(), "", errors.New("denied"))
					}
					return true, watch.NewFake(), nil
				})
				watchRVs := watchResourceVersions(mockClient)
				listCalls := pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a"}, "", nil
				})

				start(informer.DefaultNoCacheInformerOptions())

				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(4))
				Expect(listCalls.Load()).To(BeEquivalentTo(1))
				Expect(watchRVs()).To(HaveEach("10"))
			})

			It("should relist when the watch fails because the resource version expired", func() {
				var watchCalls atomic.Int32
				mockClient.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
					if watchCalls.Add(1) == 1 {
						return true, nil, apierrors.NewResourceExpired("too old resource version")
					}
					return true, watch.NewFake(), nil
				})
				listCalls := pagedLists(mockClient, func(string) ([]string, string, error) {
					return []string{"a"}, "", nil
				})

				start(informer.DefaultNoCacheInformerOptions())

				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(2))
				Expect(listCalls.Load()).To(BeEquivalentTo(2))
			})

			It("should relist at most once per backoff cap after dropped events", func() {
				current, watchCalls := fakeWatches(mockClient)
				var lock sync.Mutex
				var listTimes []time.Time
				listCount := func() int {
					lock.Lock()
					defer lock.Unlock()
					return len(listTimes)
				}
				pagedLists(mockClient, func(string) ([]string, string, error) {
					lock.Lock()
					defer lock.Unlock()
					listTimes = append(listTimes, time.Now())
					return []string{"x"}, "", nil
				})

				options := informer.DefaultNoCacheInformerOptions()
				options.QueueSize = 1
				options.WorkerCount = 1
				inf := informer.NewNoCacheWithOptions(ctx, gvr, mockClient, eventHandler, options)
				inf.SetTestTimings(wait.Backoff{Duration: 10 * time.Millisecond, Factor: 2, Steps: 3, Cap: time.Second}, 100*time.Millisecond)
				Expect(inf.Start()).To(Succeed())
				DeferCleanup(inf.Stop)
				DeferCleanup(eventHandler.Unblock)
				Eventually(eventHandler.Events, timeout, interval).Should(HaveLen(1))

				dropEvent := func() {
					eventHandler.Reset()
					eventHandler.Block()
					w := current.Load()
					w.Modify(NewUnstructured("x")) // taken by the blocked worker
					w.Modify(NewUnstructured("x")) // fills the queue
					w.Modify(NewUnstructured("x")) // dropped after the enqueue timeout
				}

				dropEvent()
				Eventually(listCount, timeout, 10*time.Millisecond).Should(Equal(2))
				eventHandler.Unblock()
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(2))
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "x")))

				dropEvent()
				Eventually(listCount, timeout, 10*time.Millisecond).Should(Equal(3))
				eventHandler.Unblock()

				lock.Lock()
				defer lock.Unlock()
				Expect(listTimes[2].Sub(listTimes[1])).To(BeNumerically(">=", 800*time.Millisecond))
			})

			It("should survive a watch error event without details", func() {
				current, watchCalls := fakeWatches(mockClient)
				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(inf.Ready, timeout, interval).Should(BeTrue())
				Eventually(current.Load, timeout, interval).ShouldNot(BeNil())

				first := current.Load()
				first.Error(&metav1.Status{Code: 500, Message: "etcdserver: request timed out"})
				first.Add(NewUnstructured("after-error"))
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "after-error")))

				// The API server closes the watch after such an error.
				first.Stop()
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(2))
				current.Load().Add(NewUnstructured("after-restart"))
				Eventually(eventHandler.Events, timeout, interval).Should(ContainElement(HaveEvent("create", "after-restart")))
			})

			It("should not reload when the watcher was stopped by a reload", func() {
				_, watchCalls := fakeWatches(mockClient)

				inf := start(informer.DefaultNoCacheInformerOptions())
				Eventually(watchCalls.Load, timeout, interval).Should(BeEquivalentTo(1))

				for range 5 {
					Expect(inf.Reload()).To(Succeed())
				}

				Consistently(watchCalls.Load, 500*time.Millisecond, 20*time.Millisecond).Should(BeEquivalentTo(6))
			})

			It("should stop retrying when the informer is stopped", func() {
				var retries atomic.Int32
				logger := funcr.New(func(_, args string) {
					if strings.Contains(args, "Failed to (re)start watcher, retrying") {
						retries.Add(1)
					}
				}, funcr.Options{})
				mockClient.PrependWatchReactor("*", func(k8stesting.Action) (bool, watch.Interface, error) {
					return true, nil, errors.New("api server unavailable")
				})

				inf := informer.NewNoCacheWithOptions(logr.NewContext(ctx, logger), gvr, mockClient, eventHandler, informer.DefaultNoCacheInformerOptions())
				inf.SetTestTimings(fastBackoff, 100*time.Millisecond)
				Expect(inf.Start()).To(Succeed())
				Eventually(retries.Load, timeout, interval).Should(BeNumerically(">=", 3))

				inf.Stop()
				stoppedAt := retries.Load()

				// An attempt that failed just before Stop may still log once.
				Consistently(retries.Load, 500*time.Millisecond, 20*time.Millisecond).Should(BeNumerically("<=", stoppedAt+1))
			})
		})

		It("should stop gracefully", func() {
			inf := informer.NewNoCache(ctx, gvr, mockClient, eventHandler)
			Expect(inf).NotTo(BeNil())
			err := inf.Start()
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				g.Expect(inf.Ready()).To(BeTrue())
			}, timeout, interval).Should(Succeed())

			inf.Stop()

		})

	})

})
