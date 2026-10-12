// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RestrictedIdentity applies the ClusterRoles and Roles of RBAC manifest files (e.g.
// config/rbac/role.yaml and leader_election_role.yaml) under names prefixed with user, binds
// them to user and returns a client impersonating user. Roles without a namespace are created
// in roleNamespace. The result reflects the effective permissions of the packaged roles.
func RestrictedIdentity(ctx context.Context, admin client.Client, cfg *rest.Config, user, roleNamespace string,
	files ...string,
) (client.WithWatch, error) {
	for _, file := range files {
		if err := applyRBACFile(ctx, admin, user, roleNamespace, file); err != nil {
			return nil, err
		}
	}
	restricted := rest.CopyConfig(cfg)
	restricted.Impersonate = rest.ImpersonationConfig{UserName: user}
	c, err := client.NewWithWatch(restricted, client.Options{Scheme: admin.Scheme()})
	if err != nil {
		return nil, fmt.Errorf("creating impersonating client: %w", err)
	}
	return c, nil
}

func applyRBACFile(ctx context.Context, admin client.Client, user, roleNamespace, file string) error {
	f, err := os.Open(file) //nolint:gosec // test helper reading repository manifests
	if err != nil {
		return fmt.Errorf("opening %s: %w", file, err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	dec := utilyaml.NewYAMLOrJSONDecoder(f, 4096)
	for {
		// Role and ClusterRole share the decoded fields (kind, metadata, rules).
		var doc rbacv1.ClusterRole
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decoding %s: %w", file, err)
		}
		name := user + "-" + doc.Name
		subject := []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: user, APIGroup: rbacv1.GroupName}}
		switch doc.Kind {
		case "ClusterRole":
			role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: doc.Rules}
			binding := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: name}, Subjects: subject,
				RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name},
			}
			if err := createAll(ctx, admin, role, binding); err != nil {
				return err
			}
		case "Role":
			ns := doc.Namespace
			if ns == "" {
				ns = roleNamespace
			}
			role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Rules: doc.Rules}
			binding := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Subjects: subject,
				RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: name},
			}
			if err := createAll(ctx, admin, role, binding); err != nil {
				return err
			}
		case "":
		default:
			return fmt.Errorf("unsupported RBAC kind %q in %s", doc.Kind, file)
		}
	}
}

func createAll(ctx context.Context, c client.Client, objs ...client.Object) error {
	for _, obj := range objs {
		if err := client.IgnoreAlreadyExists(c.Create(ctx, obj)); err != nil {
			return fmt.Errorf("creating %s: %w", obj.GetName(), err)
		}
	}
	return nil
}

// ExpectProviderCAConfigMapAccess asserts the provider-CA ConfigMap permissions of a restricted
// identity: clusterwide get/list/watch for labeled and unlabeled ConfigMaps alike (cache label
// selectors do not narrow RBAC), and create/update/patch only if writable. Delete is never allowed.
// ns must contain ConfigMaps named labeledName and unlabeledName.
func ExpectProviderCAConfigMapAccess(ctx context.Context, c client.WithWatch, ns, labeledName, unlabeledName string, writable bool) {
	for _, name := range []string{labeledName, unlabeledName} {
		cm := &corev1.ConfigMap{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, cm)).To(gomega.Succeed(), "get %s", name)
	}
	gomega.Expect(c.List(ctx, &corev1.ConfigMapList{}, client.InNamespace(ns))).To(gomega.Succeed())
	gomega.Expect(c.List(ctx, &corev1.ConfigMapList{})).To(gomega.Succeed(), "clusterwide list")
	w, err := c.Watch(ctx, &corev1.ConfigMapList{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "clusterwide watch")
	w.Stop()

	created := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rbac-probe-" + labeledName, Namespace: ns}}
	err = c.Create(ctx, created)
	if !writable {
		gomega.Expect(apierrors.IsForbidden(err)).To(gomega.BeTrue(), "create must be forbidden, got %v", err)
		existing := &corev1.ConfigMap{}
		gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: ns, Name: labeledName}, existing)).To(gomega.Succeed())
		existing.Data = map[string]string{"x": "y"}
		err = c.Update(ctx, existing)
		gomega.Expect(apierrors.IsForbidden(err)).To(gomega.BeTrue(), "update must be forbidden, got %v", err)
		err = c.Delete(ctx, existing)
		gomega.Expect(apierrors.IsForbidden(err)).To(gomega.BeTrue(), "delete must be forbidden, got %v", err)
		return
	}
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	created.Data = map[string]string{"x": "y"}
	gomega.Expect(c.Update(ctx, created)).To(gomega.Succeed())
	err = c.Delete(ctx, created)
	gomega.Expect(apierrors.IsForbidden(err)).To(gomega.BeTrue(), "delete must be forbidden, got %v", err)
}
