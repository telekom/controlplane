// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	adminv1 "github.com/telekom/controlplane/admin/api/v1"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	eventv1 "github.com/telekom/controlplane/event/api/v1"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	pubsubv1 "github.com/telekom/controlplane/pubsub/api/v1"
)

// ListenerPlacement holds the fully resolved placement resources for a Listener.
// The handler projects relevant fields from this into the compact PlacementIntent
// (defined in the handler package) for authorization fingerprinting.
type ListenerPlacement struct {
	CaptureZone        *adminv1.Zone
	CaptureRoute       *gatewayv1.Route
	CaptureEventConfig *eventv1.EventConfig
	CaptureEventStore  *pubsubv1.EventStore

	// CallbackOriginZone is the zone whose EventConfig advertises the bridge
	// callback ingress: the capture zone (see CallbackOriginZone).
	CallbackOriginZone *adminv1.Zone

	DeliveryZone        *adminv1.Zone
	DeliveryEventConfig *eventv1.EventConfig
	DeliveryEventStore  *pubsubv1.EventStore

	// BridgeNamespace is the Kubernetes namespace where bridge Subscribers and
	// the shared generic Publisher live. Derived from CaptureEventStore.Namespace.
	BridgeNamespace string

	// EnvironmentAliases are the warnings of the delivery and capture
	// environment checks for the "default" exemption. They are not part of the
	// placement intent.
	EnvironmentAliases []*EnvironmentAlias

	// CallbackBaseURL is the external gateway URL used for event delivery
	// callbacks: the delivery EventConfig's Status.CallbackURL when the origin is
	// the delivery zone, otherwise the capture EventConfig's
	// Status.ProxyCallbackURLs[delivery zone]. Either way the final
	// DynamicUpstream hop runs on A's gateway, so localhost:8080 is A's Jumper.
	CallbackBaseURL string
}

// SameZone reports whether a and b are the same Zone (name and namespace).
func SameZone(a, b *adminv1.Zone) bool {
	return a.Name == b.Name && a.Namespace == b.Namespace
}

// CandidateRejection records why a capture candidate zone was skipped.
type CandidateRejection struct {
	Zone   string
	Reason string
}

// NoCaptureCandidateError reports that no candidate zone supports capture, or
// that the first zone supporting capture cannot deliver to the observer (A)
// zone (DeliveryPath set). It is a BlockedError: the Listener waits until the
// topology changes.
type NoCaptureCandidateError struct {
	ApiBasePath string
	Rejections  []CandidateRejection
	// DeliveryPath, when set, is why the zone that supports capture cannot
	// deliver to A. Rejections then lists the zones skipped before it.
	DeliveryPath *DeliveryPathError
}

func (e *NoCaptureCandidateError) Error() string {
	parts := make([]string, 0, len(e.Rejections))
	for _, r := range e.Rejections {
		parts = append(parts, fmt.Sprintf("zone %q: %s", r.Zone, r.Reason))
	}
	if e.DeliveryPath == nil {
		return fmt.Sprintf("no supported capture zone for path %q: %s", e.ApiBasePath, strings.Join(parts, "; "))
	}
	msg := fmt.Sprintf("capture zone %q for path %q cannot deliver to observer zone %q: %s; "+
		"capture is not moved to another zone because Listeners with the same consumer, provider and path capture in one zone",
		e.DeliveryPath.CaptureZone, e.ApiBasePath, e.DeliveryPath.DeliveryZone, e.DeliveryPath.Reason)
	if len(parts) > 0 {
		msg += " (skipped " + strings.Join(parts, "; ") + ")"
	}
	return msg
}

// Unwrap returns the DeliveryPathError, if any.
func (e *NoCaptureCandidateError) Unwrap() error {
	if e.DeliveryPath == nil {
		return nil
	}
	return e.DeliveryPath
}

// IsBlocked implements ctrlerrors.BlockedError.
func (e *NoCaptureCandidateError) IsBlocked() bool { return true }

// DeliveryPathError reports that a capture zone cannot deliver to the
// observer (A) zone: the capture EventConfig's mesh excludes A's zone, it has
// no ProxyCallbackURLs entry for A's zone, or, when capture is in A's zone, the
// delivery EventConfig has no CallbackURL. It is a BlockedError and ends
// candidate evaluation: capture is never moved to another zone to reach A.
type DeliveryPathError struct {
	CaptureZone  string
	DeliveryZone string
	Reason       string
}

func (e *DeliveryPathError) Error() string { return e.Reason }

// IsBlocked implements ctrlerrors.BlockedError.
func (e *DeliveryPathError) IsBlocked() bool { return true }

// Event environments in Horizon. A zone's Jumper publishes to PUBLISH_EVENT_URL
// with ENVIRONMENT_PLACEHOLDER replaced by the zone's identity realm (pathEnv)
// and a token of that realm (gateway-jumper SpectreService.publishEvent,
// SpectreRoutingFilter). Starlight requires the token realm to equal
// alias(pathEnv) (pubsub-horizon-starlight PublisherService.checkRealm), and
// Galaxy delivers only to Subscribers whose spec.environment equals
// alias(pathEnv) (pubsub-horizon-galaxy SubscriberCache.java:55-58), logging
// nothing but INFO "No recipients found for event" otherwise. alias maps
// Horizon's defaultEnvironment to "default" and keeps every other value.
//
// The checks below compare the realm with the EventStore environment and
// assume no alias. The Control Plane cannot see Horizon's defaultEnvironment,
// so an EventStore environment of "default" with another realm is only
// reported as an EnvironmentAlias: it works when defaultEnvironment is that
// realm. Equal values can still be dropped when Galaxy's defaultEnvironment is
// that environment, or rejected by Starlight when its realm pattern
// ^https://[\w-.]+/auth/realms/(\w+)$ does not match the issuer
// (TokenServiceImpl.java:21,58-66); the checks do not detect that.

// EnvironmentMismatchError reports that a zone publishes events with another
// environment than its EventStore registers their subscriptions with, so
// Galaxy would drop every event while every condition stays Ready. It is a
// BlockedError: the Listener waits until the platform configuration changes.
type EnvironmentMismatchError struct {
	// Role is "capture" (bridge events, Jumper hop 1) or "observer" (per-app
	// events re-published at /autoevent, Jumper hop 2).
	Role               string
	Zone               string
	PublishEnvironment string
	// EventStore, EventConfig and Environment are namespace/name references.
	EventStore           string
	SubscribeEnvironment string
	SubscribeSource      string
	// EventConfig and Environment name the platform settings that decide the
	// two values: EventConfig spec.overwriteEnvironmentName (copied into the
	// EventStore) and Environment spec.realmName.
	EventConfig string
	Environment string
}

func (e *EnvironmentMismatchError) Error() string {
	return describeEnvironments(e.Role, e.Zone, e.PublishEnvironment, e.EventStore, e.SubscribeEnvironment, e.SubscribeSource) +
		fmt.Sprintf("; Galaxy would drop every event; "+
			"set EventConfig %q spec.overwriteEnvironmentName or Environment %q spec.realmName to match",
			e.EventConfig, e.Environment)
}

// IsBlocked implements ctrlerrors.BlockedError.
func (e *EnvironmentMismatchError) IsBlocked() bool { return true }

// EnvironmentAlias reports a zone that publishes events with its realm while
// its EventStore subscribes with "default". Galaxy delivers them only when
// Horizon's defaultEnvironment is that realm, which the Control Plane cannot
// verify; it is a warning, never a block.
type EnvironmentAlias struct {
	Role               string
	Zone               string
	PublishEnvironment string
	// EventStore is a namespace/name reference.
	EventStore      string
	SubscribeSource string
}

func (a *EnvironmentAlias) String() string {
	return describeEnvironments(a.Role, a.Zone, a.PublishEnvironment, a.EventStore, defaultEnvironment, a.SubscribeSource) +
		fmt.Sprintf("; events are delivered only if the Horizon defaultEnvironment of Starlight and Galaxy is %q",
			a.PublishEnvironment)
}

// describeEnvironments states the environment a zone publishes events with and
// the one its EventStore subscribes with, each with its source.
func describeEnvironments(role, zone, publishEnv, eventStore, subscribeEnv, subscribeSource string) string {
	return fmt.Sprintf("%s zone %q publishes events with environment %q (%s) "+
		"but its EventStore %q subscribes with environment %q (%s)",
		role, zone, publishEnv, publishSource, eventStore, subscribeEnv, subscribeSource)
}

// defaultEnvironment is the environment Horizon's defaultEnvironment alias maps to.
const defaultEnvironment = "default"

// publishSource names where the publish environment comes from. Both the zone's
// Status.RealmName (admin populateRealmName) and the name of its default
// identity Realm, whose issuer the RouteListener carries (admin
// naming.ForDefaultIdentityRealm), are Environment.GetRealmName().
const publishSource = "identity realm of the zone: Environment spec.realmName, else the Environment name"

// subscriptionEnvironment returns the environment the pubsub domain registers
// es's subscriptions with. It mirrors effectiveEnvironmentName in
// pubsub/internal/handler/subscriber/handler.go, which is internal to the
// pubsub module.
func subscriptionEnvironment(ctx context.Context, es *pubsubv1.EventStore) string {
	if es.Spec.OverwriteEnvironmentName != "" {
		return es.Spec.OverwriteEnvironmentName
	}
	return contextutil.EnvFromContextOrDie(ctx)
}

// subscriptionSource names where subEnv, the EventStore environment, comes
// from. The event operator copies EventConfig spec.overwriteEnvironmentName
// into the EventStore, or the Control Plane environment name when it is unset
// (effectiveEnvironmentName in event/internal/handler/eventconfig/handler.go).
// Until it has done so after an EventConfig change, subEnv differs from the
// value the EventConfig resolves to; EventConfig readiness does not cover that.
func subscriptionSource(ctx context.Context, ec *eventv1.EventConfig, subEnv string) string {
	want, source := ec.Spec.OverwriteEnvironmentName, "EventConfig spec.overwriteEnvironmentName"
	if want == "" {
		want, source = contextutil.EnvFromContextOrDie(ctx), "Control Plane environment name, copied by the event operator"
	}
	if subEnv != want {
		return fmt.Sprintf("EventStore not yet updated by the event operator, EventConfig resolves to %q", want)
	}
	return source
}

func checkEventEnvironment(
	ctx context.Context, role string, zone *adminv1.Zone, env string, ec *eventv1.EventConfig, es *pubsubv1.EventStore,
) (*EnvironmentAlias, error) {
	subEnv := subscriptionEnvironment(ctx, es)
	esRef := ctypes.ObjectRefFromObject(es).String()
	switch {
	case env == subEnv:
		return nil, nil
	case subEnv == defaultEnvironment:
		return &EnvironmentAlias{
			Role: role, Zone: zone.Name, PublishEnvironment: env, EventStore: esRef,
			SubscribeSource: subscriptionSource(ctx, ec, subEnv),
		}, nil
	}
	cpEnv := contextutil.EnvFromContextOrDie(ctx)
	return nil, &EnvironmentMismatchError{
		Role: role, Zone: zone.Name, PublishEnvironment: env, EventStore: esRef,
		SubscribeEnvironment: subEnv, SubscribeSource: subscriptionSource(ctx, ec, subEnv),
		EventConfig: ctypes.ObjectRefFromObject(ec).String(),
		// The Environment lives in the namespace of its own name.
		Environment: cpEnv + "/" + cpEnv,
	}
}

// CheckCaptureEnvironment compares the environment the capture zone's Jumper
// publishes bridge events with against es, the capture EventStore holding the
// bridge Subscribers, with ec the zone's EventConfig that owns es. The Jumper
// takes the environment from the request's realm (gateway-jumper
// SpectreService.publishEvent, jc.getRealmName()): the "realm" header that last
// mile security sets from the capture Route's spec.security.realmName, which is
// Zone.Status.RealmName. The check reads it from the RouteListener gateway client
// issuer (gatewayIssuer) instead, the text after its last "realms/". Both are
// Environment.GetRealmName(): the issuer is that of the zone's default identity
// Realm, named after it (see publishSource). It returns an
// *EnvironmentMismatchError or, for the "default" exemption, an *EnvironmentAlias.
func CheckCaptureEnvironment(
	ctx context.Context, captureZone *adminv1.Zone, gatewayIssuer string, ec *eventv1.EventConfig, es *pubsubv1.EventStore,
) (*EnvironmentAlias, error) {
	env := gatewayIssuer
	if i := strings.LastIndex(gatewayIssuer, "realms/"); i >= 0 {
		env = gatewayIssuer[i+len("realms/"):]
	}
	return checkEventEnvironment(ctx, "capture", captureZone, env, ec, es)
}

// CheckDeliveryEnvironment compares the environment the observer zone's Jumper
// re-publishes per-app events with at /autoevent against es, the observer
// EventStore holding the SpectreApplication Subscriber, with ec the zone's
// EventConfig that owns es. The Jumper takes the environment from the LMS
// token issuer realm (gateway-jumper SpectreRoutingFilter), which is the realm
// of the zone's callback Route: Zone.Status.RealmName, as the event domain sets
// it. It returns an *EnvironmentMismatchError or, for the "default" exemption,
// an *EnvironmentAlias. A zone without RealmName is not reconciled yet; that
// returns a plain BlockedError.
func CheckDeliveryEnvironment(
	ctx context.Context, observerZone *adminv1.Zone, ec *eventv1.EventConfig, es *pubsubv1.EventStore,
) (*EnvironmentAlias, error) {
	if observerZone.Status.RealmName == "" {
		return nil, ctrlerrors.BlockedErrorf("zone %q has no RealmName in status", observerZone.Name)
	}
	return checkEventEnvironment(ctx, "observer", observerZone, observerZone.Status.RealmName, ec, es)
}

// CaptureCandidateZones returns the capture candidates in evaluation order:
// the consumer (C) zone, then the provider (P) zone, deduplicated by zone
// identity. Gateway traffic from C to P only transits these two zones.
//
// The order never depends on the observer (A). Listeners that share C, P and
// path must capture in the same zone: the Jumpers of two capture zones would
// both capture the same request. When both zones feed the same Horizon, each
// Listener's bridge would accept both captures, since the bridge trigger
// carries no zone, and deliver every event twice.
func CaptureCandidateZones(consumerZone, providerZone *adminv1.Zone) []*adminv1.Zone {
	if SameZone(consumerZone, providerZone) {
		return []*adminv1.Zone{consumerZone}
	}
	return []*adminv1.Zone{consumerZone, providerZone}
}

// DeliveryPlacement is the observer (A) zone's event infrastructure. Delivery
// always happens there; it is never moved to another zone.
type DeliveryPlacement struct {
	Zone        *adminv1.Zone
	EventConfig *eventv1.EventConfig
	EventStore  *pubsubv1.EventStore
	// EnvironmentAlias, when set, is the warning CheckDeliveryEnvironment
	// returned for the "default" exemption.
	EnvironmentAlias *EnvironmentAlias
}

// ResolveDelivery resolves A's EventConfig and EventStore and checks that A's
// zone re-publishes per-app events with the environment the EventStore
// subscribes with (CheckDeliveryEnvironment). Errors are returned unwrapped so
// each caller names its own context; callers return an
// *EnvironmentMismatchError as is, since it names its role and zone. Any
// failure blocks the Listener; there is no fallback to the consumer or
// provider zone.
func ResolveDelivery(ctx context.Context, observerZone *adminv1.Zone) (*DeliveryPlacement, error) {
	ec, err := GetEventConfig(ctx, observerZone)
	if err != nil {
		return nil, err
	}
	es, err := ResolveEventStore(ctx, ec)
	if err != nil {
		return nil, err
	}
	alias, err := CheckDeliveryEnvironment(ctx, observerZone, ec, es)
	if err != nil {
		return nil, err
	}
	return &DeliveryPlacement{Zone: observerZone, EventConfig: ec, EventStore: es, EnvironmentAlias: alias}, nil
}

// CallbackOriginZone returns the zone whose EventConfig advertises the bridge
// callback ingress for events captured in captureZone: the capture zone itself.
// The bridge Subscribers live in the capture EventStore, which a proxy zone
// owns as its own CR even though its target backend's Horizon delivers them.
//
// For a local zone S, the event domain builds Status.ProxyCallbackURLs[T] as a
// Route on S's gateway whose upstream is T's gateway /horizon-T/callback/v1,
// T's primary callback Route, whose DynamicUpstream makes the final hop from
// T's gateway. A proxy zone owns no such Routes; its CallbackURL and
// ProxyCallbackURLs are projected from its target backend, so the first hop is
// the backend's gateway. Either way the origin EventConfig's entry for the
// delivery zone ends at A's gateway -> localhost:8080 at A's Jumper. This is
// the same exposure-keyed lookup the event domain uses for EventSubscriptions.
func CallbackOriginZone(captureZone *adminv1.Zone, captureEC *eventv1.EventConfig) (*adminv1.Zone, error) {
	if captureEC.Spec.Zone.Name != captureZone.Name {
		return nil, errors.Errorf("placement: EventConfig %q belongs to zone %q, not capture zone %q",
			captureEC.Name, captureEC.Spec.Zone.Name, captureZone.Name)
	}
	return captureZone, nil
}

// ResolveCaptureZone resolves capture in captureZone for delivery d. It returns
// the placement on success, a non-empty reason when the zone is definitively
// unsuitable for capture (no EventConfig), a *DeliveryPathError when the zone
// cannot deliver to A (mesh excludes the delivery zone, no callback path to A),
// or another error for transient or ambiguous state (read errors, NotReady
// EventConfig or EventStore, duplicate EventConfigs). Callers may try another
// candidate only for a reason, never for an error.
func ResolveCaptureZone(
	ctx context.Context,
	captureZone *adminv1.Zone,
	route *gatewayv1.Route,
	d *DeliveryPlacement,
) (*ListenerPlacement, string, error) {
	ec, es := d.EventConfig, d.EventStore
	if !SameZone(captureZone, d.Zone) {
		var err error
		ec, err = FindEventConfig(ctx, captureZone)
		if err != nil {
			return nil, "", errors.Wrap(err, "placement: failed to get capture EventConfig")
		}
		if ec == nil {
			return nil, fmt.Sprintf("zone %q has no EventConfig", captureZone.Name), nil
		}
		if condition.EnsureReady(ec) != nil {
			return nil, "", ctrlerrors.BlockedErrorf("placement: capture EventConfig %q for zone %q is not ready",
				ec.Name, captureZone.Name)
		}
		es, err = ResolveEventStore(ctx, ec)
		if err != nil {
			return nil, "", errors.Wrap(err, "placement: failed to resolve capture EventStore")
		}
	}

	if !ec.SupportsZone(d.Zone.Name) {
		return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("EventConfig %q of zone %q does not mesh with delivery zone %q",
			ec.Name, captureZone.Name, d.Zone.Name)}
	}

	origin, err := CallbackOriginZone(captureZone, ec)
	if err != nil {
		return nil, "", err
	}
	var base string
	if SameZone(origin, d.Zone) {
		base = d.EventConfig.Status.CallbackURL
		if base == "" {
			return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("delivery EventConfig %q has no CallbackURL in status", d.EventConfig.Name)}
		}
	} else {
		base = ec.Status.ProxyCallbackURLs[d.Zone.Name]
		if base == "" {
			return nil, "", &DeliveryPathError{CaptureZone: captureZone.Name, DeliveryZone: d.Zone.Name, Reason: fmt.Sprintf("capture EventConfig %q has no ProxyCallbackURLs entry for delivery zone %q",
				ec.Name, d.Zone.Name)}
		}
	}

	return &ListenerPlacement{
		CaptureZone:         captureZone,
		CaptureRoute:        route,
		CaptureEventConfig:  ec,
		CaptureEventStore:   es,
		CallbackOriginZone:  origin,
		DeliveryZone:        d.Zone,
		DeliveryEventConfig: d.EventConfig,
		DeliveryEventStore:  d.EventStore,
		BridgeNamespace:     es.Namespace,
		CallbackBaseURL:     base,
		EnvironmentAliases:  appendAlias(nil, d.EnvironmentAlias),
	}, "", nil
}

// appendAlias appends a to aliases unless it is nil.
func appendAlias(aliases []*EnvironmentAlias, a *EnvironmentAlias) []*EnvironmentAlias {
	if a == nil {
		return aliases
	}
	return append(aliases, a)
}
