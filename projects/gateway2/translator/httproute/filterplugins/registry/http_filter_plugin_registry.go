package registry

import (
	"fmt"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/extensionref"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/mirror"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/redirect"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/routeoptions"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/urlrewrite"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/headermodifier"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPFilterPluginRegistry interface {
	GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error)
	GetExtensionPlugin(gk schema.GroupKind) (filterplugins.ExtPlugin, error)
}

type httpFilterPluginRegistry struct {
	standardPlugins  map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin
	extensionPlugins map[schema.GroupKind]filterplugins.ExtPlugin
}

func (h *httpFilterPluginRegistry) GetStandardPlugin(filterType gwv1.HTTPRouteFilterType) (filterplugins.FilterPlugin, error) {
	p, ok := h.standardPlugins[filterType]
	if !ok {
		return nil, fmt.Errorf("")
	}
	return p, nil
}

func (h *httpFilterPluginRegistry) GetExtensionPlugin(gk schema.GroupKind) (filterplugins.ExtPlugin, error) {
	p, ok := h.extensionPlugins[schema.GroupKind{
		Group: string(gk.Group),
		Kind:  string(gk.Kind),
	}]
	if !ok {
		return nil, fmt.Errorf("no extenstion support for gk %+v", gk)
	}
	return p, nil
}

func NewHTTPFilterPluginRegistry(queries query.GatewayQueries) HTTPFilterPluginRegistry {
	reg := &httpFilterPluginRegistry{
		standardPlugins: map[gwv1.HTTPRouteFilterType]filterplugins.FilterPlugin{
			gwv1.HTTPRouteFilterRequestHeaderModifier:  headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterResponseHeaderModifier: headermodifier.NewPlugin(),
			gwv1.HTTPRouteFilterURLRewrite:             urlrewrite.NewPlugin(),
			gwv1.HTTPRouteFilterRequestRedirect:        redirect.NewPlugin(),
			gwv1.HTTPRouteFilterRequestMirror:          mirror.NewPlugin(),
		},
		extensionPlugins: map[schema.GroupKind]filterplugins.ExtPlugin{
			{
				Group: sologatewayv1.RouteOptionGVK.Group,
				Kind:  sologatewayv1.RouteOptionGVK.Kind,
			}: routeoptions.NewPlugin(),
		},
	}
	getPlugin := func(gk schema.GroupKind) (filterplugins.ExtPlugin, error) {
		return reg.GetExtensionPlugin(gk)
	}
	reg.standardPlugins[gwv1.HTTPRouteFilterExtensionRef] = extensionref.NewPlugin(queries, getPlugin)
	return reg
}
