package registry

import (
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
)

type RoutePluginRegistry struct {
	routePlugins []plugins.RoutePlugin
}

func (h *RoutePluginRegistry) GetRoutePlugins() []plugins.RoutePlugin {
	return h.routePlugins
}

func NewRoutePluginRegistry(
	queries query.GatewayQueries,
) *RoutePluginRegistry {
	filter := filterplugins.NewRouteFilterPlugin(queries)
	return &RoutePluginRegistry{
		routePlugins: []plugins.RoutePlugin{
			&filter,
		},
	}
}
