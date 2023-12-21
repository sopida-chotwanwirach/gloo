package registry

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/api"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/headermodifier"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/urlrewrite"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPFilterPluginRegistry struct {
	filterPlugins map[gwv1.HTTPRouteFilterType]api.HTTPFilterPlugin
}

func (h *HTTPFilterPluginRegistry) GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (api.HTTPFilterPlugin, error) {
	p, ok := h.filterPlugins[filterType]
	if !ok {
		return nil, fmt.Errorf("")
	}
	return p, nil
}

func NewHTTPFilterPluginRegistry(
	queries query.GatewayQueries,
) *HTTPFilterPluginRegistry {
	return &HTTPFilterPluginRegistry{
		filterPlugins: map[gwv1.HTTPRouteFilterType]api.HTTPFilterPlugin{
			gwv1.HTTPRouteFilterRequestHeaderModifier:  headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterResponseHeaderModifier: headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterURLRewrite:             urlrewrite.NewPlugin(),
			gwv1.HTTPRouteFilterRequestRedirect:        redirect.NewPlugin(),
			gwv1.HTTPRouteFilterRequestMirror:          mirror.NewPlugin(queries),
			// gwv1.HTTPRouteFilterExtensionRef:           extensionref.NewPlugin(queries, extRegistry),
		},
	}
}
