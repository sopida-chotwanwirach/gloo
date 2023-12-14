package extensions

import (
	"fmt"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/routeoptions"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ExtensionPluginRegistry struct {
	extensionPlugins map[schema.GroupKind]filterplugins.ExtensionPlugin
}

func NewExtensionPluginRegistry() *ExtensionPluginRegistry {
	return &ExtensionPluginRegistry{
		extensionPlugins: map[schema.GroupKind]filterplugins.ExtensionPlugin{
			{
				Group: sologatewayv1.RouteOptionGVK.Group,
				Kind:  sologatewayv1.RouteOptionGVK.Kind,
			}: routeoptions.NewPlugin(),
		},
	}
}

func (h *ExtensionPluginRegistry) GetExtensionPlugin(gk schema.GroupKind) (filterplugins.ExtensionPlugin, error) {
	p, ok := h.extensionPlugins[schema.GroupKind{
		Group: string(gk.Group),
		Kind:  string(gk.Kind),
	}]
	if !ok {
		return nil, fmt.Errorf("no extenstion support for gk %+v", gk)
	}
	return p, nil
}
