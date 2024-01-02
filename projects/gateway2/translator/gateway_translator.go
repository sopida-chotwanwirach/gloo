package translator

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/registry"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/listener"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// K8sGwTranslator This translator Translates K8s Gateway resources into Gloo Edege Proxies.
type K8sGwTranslator interface {
	// TranslateProxy This function is called by the reconciler when a K8s Gateway resource is created or updated.
	// It returns an instance of the Gloo Edge Proxy resources that should configure a target Gloo Edge Proxy.
	// A null return value indicates the K8s Gateway resource failed to translate into a Gloo Edge Proxy. The error will be reported on the provided reporter.
	TranslateProxy(
		ctx context.Context,
		gateway *gwv1.Gateway,
		queries query.GatewayQueries,
		reporter reports.Reporter,
	) *v1.Proxy
}

func NewTranslator(
	routePlugins registry.RoutePluginRegistry,
) K8sGwTranslator {
	return &translator{
		routePlugins,
	}
}

type translator struct {
	routePlugins registry.RoutePluginRegistry
}

func (t *translator) TranslateProxy(
	ctx context.Context,
	gateway *gwv1.Gateway,
	queries query.GatewayQueries,
	reporter reports.Reporter,
) *v1.Proxy {

	routesForGw, err := queries.GetRoutesForGw(ctx, gateway)
	if err != nil {
		// TODO(ilackarms): fill in the specific error / validation
		// reporter.Gateway(gateway).Err(err.Error())
		return nil
	}
	for _, rErr := range routesForGw.RouteErrors {
		reporter.Route(&rErr.Route).ParentRef(&rErr.ParentRef).SetCondition(reports.HTTPRouteCondition{
			Type:   gwv1.RouteConditionAccepted,
			Status: metav1.ConditionFalse,
			Reason: rErr.Error.Reason,
			// TODO message
		})
	}

	for _, listener := range gateway.Spec.Listeners {
		availRoutes := 0
		if res, ok := routesForGw.ListenerResults[string(listener.Name)]; ok {
			availRoutes = len(res.Routes)
		}
		reporter.Gateway(gateway).Listener(&listener).SetAttachedRoutes(uint(availRoutes))
	}

	listeners := listener.TranslateListeners(
		ctx,
		queries,
		t.routePlugins,
		gateway,
		routesForGw,
		reporter,
	)

	return &v1.Proxy{
		Metadata:  proxyMetadata(gateway),
		Listeners: listeners,
	}
}

func proxyMetadata(gateway *gwv1.Gateway) *core.Metadata {
	// TODO(ilackarms) what should the proxy ID be
	// ROLE ON ENVOY MUST MATCH <proxy_namespace>~<proxy_name>
	// equal to role: {{.Values.settings.writeNamespace | default .Release.Namespace }}~{{ $name | kebabcase }}
	return &core.Metadata{
		Name:      gateway.Name,
		Namespace: gateway.Namespace,
	}
}
