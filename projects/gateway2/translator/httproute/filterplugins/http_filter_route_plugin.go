package filterplugins

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type routeFilterPlugin struct {
	plugins registry.HTTPFilterPluginRegistry
}

func NewRouteFilterPlugin(queries query.GatewayQueries) routeFilterPlugin {
	filterPluginRegistry := registry.NewHTTPFilterPluginRegistry(queries)
	return routeFilterPlugin{
		plugins: *filterPluginRegistry,
	}
}

func (x *routeFilterPlugin) ApplyPlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	var err error
	for _, filter := range routeCtx.Rule.Filters {
		plugin, err := x.plugins.GetStandardPlugin(filter.Type)
		if err != nil {
			// TODO log?
			break
		}
		if err := plugin.ApplyFilter(ctx, routeCtx, filter, outputRoute); err != nil {
			// TODO log?
			break
		}
	}
	if err != nil {
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:   gwv1.RouteConditionPartiallyInvalid,
			Status: metav1.ConditionTrue,
			Reason: gwv1.RouteReasonIncompatibleFilters, //TODO(Law): use this reason for all errors??
			// Message: "fill in here",
		})
	}
	return err
}
