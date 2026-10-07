// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/telekom/controlplane/common/pkg/config"
	notificationv1 "github.com/telekom/controlplane/notification/api/v1"
	organizationv1 "github.com/telekom/controlplane/organization/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	mappingNamespace = "mapping-other"
	mappingEnvA      = "mapping-env-a"
	mappingEnvB      = "mapping-env-b"
)

// newMappingTeam creates a Team that references a non-existent Group, so the
// controller blocks early and never overwrites the manually set status.namespace.
func newMappingTeam(name, namespace, env, group, statusNamespace string) *organizationv1.Team {
	team := &organizationv1.Team{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{config.EnvironmentLabelKey: env},
		},
		Spec: organizationv1.TeamSpec{
			Name:     name,
			Group:    group,
			Email:    "mapping@example.com",
			Members:  []organizationv1.Member{{Name: "member", Email: "member@example.com"}},
			Category: organizationv1.TeamCategoryCustomer,
		},
	}
	Expect(k8sClient.Create(ctx, team)).To(Succeed())
	if statusNamespace != "" {
		setTeamStatusNamespace(team, statusNamespace)
	}
	return team
}

func setTeamStatusNamespace(team *organizationv1.Team, statusNamespace string) {
	Eventually(func(g Gomega) {
		current := &organizationv1.Team{}
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(team), current)).To(Succeed())
		current.Status.Namespace = statusNamespace
		g.Expect(k8sClient.Status().Update(ctx, current)).To(Succeed())
	}, timeout, interval).Should(Succeed())
	team.Status.Namespace = statusNamespace
}

// expectTeamsCached waits until the cache-backed client sees every fixture Team in its
// expected state, so mapping assertions (including exclusions) run against a synced cache.
func expectTeamsCached(teams ...*organizationv1.Team) {
	Eventually(func(g Gomega) {
		for _, expected := range teams {
			cached := &organizationv1.Team{}
			g.Expect(k8sCachedClient.Get(ctx, client.ObjectKeyFromObject(expected), cached)).To(Succeed())
			g.Expect(cached.Labels).To(HaveKeyWithValue(config.EnvironmentLabelKey, expected.Labels[config.EnvironmentLabelKey]))
			g.Expect(cached.Spec.Group).To(Equal(expected.Spec.Group))
			g.Expect(cached.Status.Namespace).To(Equal(expected.Status.Namespace))
		}
	}, timeout, interval).Should(Succeed())
}

func requestFor(team *organizationv1.Team) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: team.Name, Namespace: team.Namespace}}
}

func newMappingChannel(name, namespace, env string) *notificationv1.NotificationChannel {
	return &notificationv1.NotificationChannel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{config.EnvironmentLabelKey: env},
		},
	}
}

func newMappingGroup(name, namespace string) *organizationv1.Group {
	return &organizationv1.Group{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
}

var _ = Describe("Team Controller mappings", Ordered, func() {
	var reconciler *TeamReconciler

	BeforeAll(func() {
		reconciler = &TeamReconciler{Client: k8sCachedClient}
		CreateNamespace(mappingNamespace)
	})

	Context("mapNotificationChannelToTeam", func() {
		It("maps a channel to all Teams with matching status namespace and environment", func() {
			statusNs := mappingEnvA + "--grp-chan--team"
			teamDefault := newMappingTeam("chan-team-default", testNamespace, mappingEnvA, "grp-chan-missing", statusNs)
			teamOther := newMappingTeam("chan-team-other", mappingNamespace, mappingEnvA, "grp-chan-missing", statusNs)
			teamWrongEnv := newMappingTeam("chan-team-wrong-env", testNamespace, mappingEnvB, "grp-chan-missing", statusNs)
			teamWrongNs := newMappingTeam("chan-team-wrong-ns", testNamespace, mappingEnvA, "grp-chan-missing", "unrelated-ns")
			expectTeamsCached(teamDefault, teamOther, teamWrongEnv, teamWrongNs)

			channel := newMappingChannel("arbitrary-channel-name", statusNs, mappingEnvA)

			Eventually(func() []reconcile.Request {
				return reconciler.mapNotificationChannelToTeam(ctx, channel)
			}, timeout, interval).Should(ConsistOf(requestFor(teamDefault), requestFor(teamOther)))

			channelOtherEnv := newMappingChannel("arbitrary-channel-name", statusNs, "mapping-env-unknown")
			Expect(reconciler.mapNotificationChannelToTeam(ctx, channelOtherEnv)).To(BeEmpty())
		})

		It("follows status namespace updates of a Team", func() {
			team := newMappingTeam("chan-team-moving", testNamespace, mappingEnvA, "grp-chan-missing", "chan-before")

			before := newMappingChannel("ch", "chan-before", mappingEnvA)
			after := newMappingChannel("ch", "chan-after", mappingEnvA)

			expectTeamsCached(team)
			Eventually(func() []reconcile.Request {
				return reconciler.mapNotificationChannelToTeam(ctx, before)
			}, timeout, interval).Should(ConsistOf(requestFor(team)))

			setTeamStatusNamespace(team, "chan-after")
			expectTeamsCached(team)

			Eventually(func(g Gomega) {
				g.Expect(reconciler.mapNotificationChannelToTeam(ctx, after)).To(ConsistOf(requestFor(team)))
				g.Expect(reconciler.mapNotificationChannelToTeam(ctx, before)).To(BeEmpty())
			}, timeout, interval).Should(Succeed())
		})

		It("returns no requests when nothing matches or the object has the wrong type", func() {
			Expect(reconciler.mapNotificationChannelToTeam(ctx, newMappingChannel("ch", "no-such-ns", mappingEnvA))).To(BeEmpty())
			Expect(reconciler.mapNotificationChannelToTeam(ctx, newMappingGroup("g", mappingEnvA))).To(BeNil())
		})
	})

	Context("mapGroupToTeam", func() {
		It("maps a Group only to Teams of its environment, across Team metadata namespaces", func() {
			teamDefault := newMappingTeam("grp-team-default", testNamespace, mappingEnvA, "grp-shared", "")
			teamOther := newMappingTeam("grp-team-other", mappingNamespace, mappingEnvA, "grp-shared", "")
			teamEnvB := newMappingTeam("grp-team-env-b", testNamespace, mappingEnvB, "grp-shared", "")
			teamDifferentGroup := newMappingTeam("grp-team-different-group", testNamespace, mappingEnvA, "grp-different", "")
			expectTeamsCached(teamDefault, teamOther, teamEnvB, teamDifferentGroup)

			Eventually(func() []reconcile.Request {
				return reconciler.mapGroupToTeam(ctx, newMappingGroup("grp-shared", mappingEnvA))
			}, timeout, interval).Should(ConsistOf(requestFor(teamDefault), requestFor(teamOther)))

			Eventually(func() []reconcile.Request {
				return reconciler.mapGroupToTeam(ctx, newMappingGroup("grp-shared", mappingEnvB))
			}, timeout, interval).Should(ConsistOf(requestFor(teamEnvB)))
		})

		It("returns no requests when nothing matches or the object has the wrong type", func() {
			Expect(reconciler.mapGroupToTeam(ctx, newMappingGroup("grp-unknown", mappingEnvA))).To(BeEmpty())
			Expect(reconciler.mapGroupToTeam(ctx, newMappingChannel("ch", mappingEnvA, mappingEnvA))).To(BeNil())
		})
	})
})
