// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	approvalv1 "github.com/telekom/controlplane/approval/api/v1"
	spectrev1 "github.com/telekom/controlplane/spectre/api/v1"
)

// TestLegacyFixtureLoader validates that the Phase 0 upgrade snapshots are
// schema-valid, can be created in envtest, and have consistent cross-references.
// It does NOT run reconciliation — it only proves the fixture graph is loadable
// and internally consistent.
func TestLegacyFixtureLoader(t *testing.T) {
	// Register CRD types on the scheme.
	if err := spectrev1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("register spectre scheme: %v", err)
	}
	if err := approvalv1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("register approval scheme: %v", err)
	}

	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "approval", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s",
			fmt.Sprintf("1.32.0-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start envtest: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Logf("stop envtest: %v", err)
		}
	})

	k8s, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	ctx := context.Background()

	// Create the namespace used by fixtures.
	ns := &unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")
	ns.SetName("fixture-team")
	if err := k8s.Create(ctx, ns); err != nil {
		t.Fatalf("create namespace: %v", err)
	}

	for _, tc := range []struct {
		name    string
		file    string
		granted bool
	}{
		{"same-team-granted", "testdata/same-team-granted.yaml", true},
		{"same-team-revoked", "testdata/same-team-revoked.yaml", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := loadFixture(t, tc.file)
			if len(objs) == 0 {
				t.Fatal("no objects in fixture")
			}

			// Phase 1: Create objects in dependency order and collect UID map.
			// Status fields are saved before creation, then restored via the
			// status subresource (the API server strips status on Create).
			uidMap := make(map[string]types.UID) // symbolic -> real
			created := make([]client.Object, 0, len(objs))
			savedOwnerRefs := make(map[string][]metav1.OwnerReference) // object name -> original ownerRefs

			for _, obj := range objs {
				symbolicUID := string(obj.GetUID())
				// Strip symbolic UID before creation (API assigns real UID).
				obj.SetUID("")
				// Save ownerReferences before stripping (will be restored with real UIDs).
				if refs := obj.GetOwnerReferences(); len(refs) > 0 {
					savedOwnerRefs[obj.GetName()] = refs
				}
				obj.SetOwnerReferences(nil)
				// Strip resourceVersion.
				obj.SetResourceVersion("")

				// Save status before creation (API drops it on Create).
				savedStatus := saveStatus(obj)

				if err := k8s.Create(ctx, obj); err != nil {
					t.Fatalf("create %s %s/%s: %v",
						obj.GetObjectKind().GroupVersionKind().Kind,
						obj.GetNamespace(), obj.GetName(), err)
				}

				// Re-fetch to get server-assigned metadata.
				key := client.ObjectKeyFromObject(obj)
				if err := k8s.Get(ctx, key, obj); err != nil {
					t.Fatalf("get %s %s/%s: %v",
						obj.GetObjectKind().GroupVersionKind().Kind,
						obj.GetNamespace(), obj.GetName(), err)
				}

				// Restore status via the status subresource.
				restoreStatus(obj, savedStatus)
				if err := k8s.Status().Update(ctx, obj); err != nil {
					t.Fatalf("update status %s %s/%s: %v",
						obj.GetObjectKind().GroupVersionKind().Kind,
						obj.GetNamespace(), obj.GetName(), err)
				}

				// Re-fetch after status update.
				if err := k8s.Get(ctx, key, obj); err != nil {
					t.Fatalf("re-get %s %s/%s: %v",
						obj.GetObjectKind().GroupVersionKind().Kind,
						obj.GetNamespace(), obj.GetName(), err)
				}

				if symbolicUID != "" {
					uidMap[symbolicUID] = obj.GetUID()
				}
				created = append(created, obj)
			}

			// Phase 1b: Remap symbolic UIDs to real UIDs and restore ownerReferences.
			for _, obj := range created {
				needsUpdate := false

				// Remap Spec.Target.UID for Approval and ApprovalRequest objects.
				switch o := obj.(type) {
				case *approvalv1.Approval:
					if realUID, ok := uidMap[string(o.Spec.Target.UID)]; ok {
						o.Spec.Target.UID = realUID
						needsUpdate = true
					}
				case *approvalv1.ApprovalRequest:
					if realUID, ok := uidMap[string(o.Spec.Target.UID)]; ok {
						o.Spec.Target.UID = realUID
						needsUpdate = true
					}
				}

				// Restore ownerReferences with remapped UIDs.
				if refs, ok := savedOwnerRefs[obj.GetName()]; ok {
					remapped := make([]metav1.OwnerReference, len(refs))
					for i, ref := range refs {
						remapped[i] = ref
						if realUID, ok := uidMap[string(ref.UID)]; ok {
							remapped[i].UID = realUID
						}
					}
					obj.SetOwnerReferences(remapped)
					needsUpdate = true
				}

				if needsUpdate {
					if err := k8s.Update(ctx, obj); err != nil {
						t.Fatalf("remap UIDs %s %s/%s: %v",
							obj.GetObjectKind().GroupVersionKind().Kind,
							obj.GetNamespace(), obj.GetName(), err)
					}
					// Re-fetch after update.
					key := client.ObjectKeyFromObject(obj)
					if err := k8s.Get(ctx, key, obj); err != nil {
						t.Fatalf("re-get after remap %s %s/%s: %v",
							obj.GetObjectKind().GroupVersionKind().Kind,
							obj.GetNamespace(), obj.GetName(), err)
					}
				}
			}

			// Phase 2: Validate cross-references.
			var listener *spectrev1.Listener
			var spectreApp *spectrev1.SpectreApplication
			var approval *approvalv1.Approval
			var ar *approvalv1.ApprovalRequest

			for _, obj := range created {
				switch o := obj.(type) {
				case *spectrev1.Listener:
					listener = o
				case *spectrev1.SpectreApplication:
					spectreApp = o
				case *approvalv1.Approval:
					approval = o
				case *approvalv1.ApprovalRequest:
					ar = o
				}
			}

			if listener == nil {
				t.Fatal("fixture missing Listener")
			}
			if spectreApp == nil {
				t.Fatal("fixture missing SpectreApplication")
			}
			if approval == nil {
				t.Fatal("fixture missing Approval")
			}
			if ar == nil {
				t.Fatal("fixture missing ApprovalRequest")
			}

			// Validate Listener -> SpectreApplication reference.
			if listener.Spec.Application.Name != spectreApp.Name {
				t.Errorf("Listener.spec.application.name = %q, want SpectreApp name %q",
					listener.Spec.Application.Name, spectreApp.Name)
			}

			// Validate Approval name follows legacy convention: listener--<listener-name>.
			wantApprovalName := "listener--" + listener.Name
			if approval.Name != wantApprovalName {
				t.Errorf("Approval name = %q, want legacy name %q", approval.Name, wantApprovalName)
			}

			// Validate Approval target references the Listener.
			if approval.Spec.Target.Name != listener.Name {
				t.Errorf("Approval.spec.target.name = %q, want %q", approval.Spec.Target.Name, listener.Name)
			}
			if approval.Spec.Target.Kind != "Listener" {
				t.Errorf("Approval.spec.target.kind = %q, want Listener", approval.Spec.Target.Kind)
			}

			// Validate Approval.spec.target.uid references the real Listener UID.
			if approval.Spec.Target.UID != listener.GetUID() {
				t.Errorf("Approval.spec.target.uid = %q, want real Listener UID %q",
					approval.Spec.Target.UID, listener.GetUID())
			}

			// Validate AR references the correct Approval.
			if ar.Status.Approval.Name != approval.Name {
				t.Errorf("AR.status.approval.name = %q, want %q", ar.Status.Approval.Name, approval.Name)
			}

			// Validate Approval -> AR approved-request reference.
			if approval.Spec.ApprovedRequest == nil {
				t.Error("Approval.spec.approvedRequest is nil")
			} else if approval.Spec.ApprovedRequest.Name != ar.Name {
				t.Errorf("Approval.spec.approvedRequest.name = %q, want %q",
					approval.Spec.ApprovedRequest.Name, ar.Name)
			}

			// Validate same-team Auto strategy.
			if approval.Spec.Strategy != approvalv1.ApprovalStrategyAuto {
				t.Errorf("Approval.spec.strategy = %q, want Auto", approval.Spec.Strategy)
			}
			if ar.Spec.Strategy != approvalv1.ApprovalStrategyAuto {
				t.Errorf("AR.spec.strategy = %q, want Auto", ar.Spec.Strategy)
			}

			// Validate requester == decider (same team).
			if approval.Spec.Requester.TeamName != approval.Spec.Decider.TeamName {
				t.Errorf("same-team fixture: requester team %q != decider team %q",
					approval.Spec.Requester.TeamName, approval.Spec.Decider.TeamName)
			}

			// Validate the System auto-approval decision.
			if len(approval.Spec.Decisions) != 1 {
				t.Fatalf("expected 1 decision, got %d", len(approval.Spec.Decisions))
			}
			if approval.Spec.Decisions[0].Name != approvalv1.SystemDecisionName {
				t.Errorf("decision name = %q, want %q",
					approval.Spec.Decisions[0].Name, approvalv1.SystemDecisionName)
			}
			if approval.Spec.Decisions[0].Comment != approvalv1.AutoApprovedComment {
				t.Errorf("decision comment = %q, want %q",
					approval.Spec.Decisions[0].Comment, approvalv1.AutoApprovedComment)
			}

			// Validate granted vs revoked states.
			if tc.granted {
				if approval.Spec.State != approvalv1.ApprovalStateGranted {
					t.Errorf("granted fixture: Approval.spec.state = %q, want Granted", approval.Spec.State)
				}
				if ar.Spec.State != approvalv1.ApprovalStateGranted {
					t.Errorf("granted fixture: AR.spec.state = %q, want Granted", ar.Spec.State)
				}
			} else {
				if approval.Spec.State != approvalv1.ApprovalStateSuspended {
					t.Errorf("revoked fixture: Approval.spec.state = %q, want Suspended", approval.Spec.State)
				}
				if approval.Status.LastState != approvalv1.ApprovalStateGranted {
					t.Errorf("revoked fixture: Approval.status.lastState = %q, want Granted (was granted before suspension)",
						approval.Status.LastState)
				}
				// The AR was granted before the Approval was suspended.
				if ar.Spec.State != approvalv1.ApprovalStateGranted {
					t.Errorf("revoked fixture: AR.spec.state = %q, want Granted (AR was granted before Approval suspension)",
						ar.Spec.State)
				}
			}

			// Validate providerApproval status reference on Listener.
			if listener.Status.ProviderApproval == nil {
				t.Error("Listener.status.providerApproval is nil")
			} else if listener.Status.ProviderApproval.Name != approval.Name {
				t.Errorf("Listener.status.providerApproval.name = %q, want %q",
					listener.Status.ProviderApproval.Name, approval.Name)
			}

			// Phase 3: Cleanup (envtest has no GC).
			for i := len(created) - 1; i >= 0; i-- {
				if err := k8s.Delete(ctx, created[i]); err != nil {
					t.Logf("cleanup %s: %v", created[i].GetName(), err)
				}
			}
		})
	}
}

// loadFixture reads a multi-document YAML file and returns typed objects.
func loadFixture(t *testing.T, path string) []client.Object {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	var objects []client.Object
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(splitYAMLDocuments)

	for scanner.Scan() {
		doc := strings.TrimSpace(scanner.Text())
		if doc == "" || doc == "---" {
			continue
		}
		// Skip comment-only documents.
		allComments := true
		for _, line := range strings.Split(doc, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				allComments = false
				break
			}
		}
		if allComments {
			continue
		}

		obj, gvk, err := scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			t.Fatalf("decode YAML document (gvk hint: %v): %v\nDocument:\n%s", gvk, err, doc[:min(200, len(doc))])
		}

		cObj, ok := obj.(client.Object)
		if !ok {
			t.Fatalf("object %T does not implement client.Object", obj)
		}
		objects = append(objects, cObj)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture %s: %v", path, err)
	}

	return objects
}

// splitYAMLDocuments is a bufio.SplitFunc that splits on YAML document
// separators ("---" on its own line).
func splitYAMLDocuments(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	sep := []byte("\n---")
	if i := bytes.Index(data, sep); i >= 0 {
		return i + len(sep), data[:i], nil
	}

	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// statusSnapshot holds a copy of the status fields from a fixture object.
type statusSnapshot struct {
	listenerStatus *spectrev1.ListenerStatus
	spectreStatus  *spectrev1.SpectreApplicationStatus
	approvalStatus *approvalv1.ApprovalStatus
	arStatus       *approvalv1.ApprovalRequestStatus
}

// saveStatus captures status fields before creation (the API strips them).
func saveStatus(obj client.Object) statusSnapshot {
	var snap statusSnapshot
	switch o := obj.(type) {
	case *spectrev1.Listener:
		s := o.Status
		snap.listenerStatus = &s
		o.Status = spectrev1.ListenerStatus{}
	case *spectrev1.SpectreApplication:
		s := o.Status
		snap.spectreStatus = &s
		o.Status = spectrev1.SpectreApplicationStatus{}
	case *approvalv1.Approval:
		s := o.Status
		snap.approvalStatus = &s
		o.Status = approvalv1.ApprovalStatus{}
	case *approvalv1.ApprovalRequest:
		s := o.Status
		snap.arStatus = &s
		o.Status = approvalv1.ApprovalRequestStatus{}
	}
	return snap
}

// restoreStatus writes the saved status back onto the object.
func restoreStatus(obj client.Object, snap statusSnapshot) {
	switch o := obj.(type) {
	case *spectrev1.Listener:
		if snap.listenerStatus != nil {
			o.Status = *snap.listenerStatus
		}
	case *spectrev1.SpectreApplication:
		if snap.spectreStatus != nil {
			o.Status = *snap.spectreStatus
		}
	case *approvalv1.Approval:
		if snap.approvalStatus != nil {
			o.Status = *snap.approvalStatus
		}
	case *approvalv1.ApprovalRequest:
		if snap.arStatus != nil {
			o.Status = *snap.arStatus
		}
	}
}
