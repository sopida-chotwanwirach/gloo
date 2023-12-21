package filterplugins

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type routeFilterPlugin struct {
	plugins registry.HTTPFilterPluginRegistry
}

func NewRouteFilterPlugin(plugins registry.HTTPFilterPluginRegistry) routeFilterPlugin {
	return routeFilterPlugin{
		plugins,
	}
}

func (x *routeFilterPlugin) ApplyPlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	queries query.GatewayQueries,
	outputRoute *v1.Route,
) error {
	for _, filter := range routeCtx.Rule.Filters {
		plugin, err := x.plugins.GetStandardPlugin(filter.Type)
		if err != nil {
			return err
		}
		if err := plugin.ApplyFilter(ctx, routeCtx, filter, outputRoute); err != nil {
			return err
		}
	}
	// if err != nil {
	// 	routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
	// 		Type:   gwv1.RouteConditionPartiallyInvalid,
	// 		Status: metav1.ConditionTrue,
	// 		Reason: gwv1.RouteReasonIncompatibleFilters, //TODO(Law): use this reason for all errors??
	// 	})
	// 	//TODO(Law): should we log these errors? we also need to propagate the error message to the condition msg
	// }
	return nil
}
