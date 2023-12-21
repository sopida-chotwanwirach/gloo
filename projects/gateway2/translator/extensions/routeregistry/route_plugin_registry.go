package routeregistry

import (
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions/filterplugins/registry"
)

type RoutePluginRegistry struct {
	routePlugins []extensions.RoutePlugin
}

func (h *RoutePluginRegistry) GetRoutePlugins() []extensions.RoutePlugin {
	return h.routePlugins
}

func NewRoutePluginRegistry(
	queries query.GatewayQueries,
) *RoutePluginRegistry {
	filter := filterplugins.NewRouteFilterPlugin(*registry.NewHTTPFilterPluginRegistry(queries))
	return &RoutePluginRegistry{
		routePlugins: []extensions.RoutePlugin{
			&filter,
		},
	}
}
