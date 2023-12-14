package registry

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/extensionref"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/urlrewrite"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/headermodifier"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPFilterPluginRegistry interface {
	GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error)
}

type httpFilterPluginRegistry struct {
	standardPlugins map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin
}

func (h *httpFilterPluginRegistry) GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error) {
	p, ok := h.standardPlugins[filterType]
	if !ok {
		return nil, fmt.Errorf("")
	}
	return p, nil
}

func NewHTTPFilterPluginRegistry(
	queries query.GatewayQueries,
	extRegistry *extensions.ExtensionPluginRegistry,
) HTTPFilterPluginRegistry {
	return &httpFilterPluginRegistry{
		standardPlugins: map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin{
			gwv1.HTTPRouteFilterRequestHeaderModifier:  headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterResponseHeaderModifier: headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterURLRewrite:             urlrewrite.NewPlugin(),
			gwv1.HTTPRouteFilterRequestRedirect:        redirect.NewPlugin(),
			gwv1.HTTPRouteFilterRequestMirror:          mirror.NewPlugin(),
			gwv1.HTTPRouteFilterExtensionRef:           extensionref.NewPlugin(queries, extRegistry),
		},
	}
}
