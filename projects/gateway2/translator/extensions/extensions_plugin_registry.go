package extensions

import (
	"context"
	"fmt"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions/routeoptions"
	sologloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ExtensionPlugin interface {
	ApplyExtPlugin(
		ctx context.Context,
		cfg client.Object,
		outputRoute *sologloov1.Route,
	) error
}

type ExtensionPluginRegistry struct {
	extensionPlugins map[schema.GroupKind]ExtensionPlugin
}

func NewExtensionPluginRegistry() *ExtensionPluginRegistry {
	return &ExtensionPluginRegistry{
		extensionPlugins: map[schema.GroupKind]ExtensionPlugin{
			{
				Group: sologatewayv1.RouteOptionGVK.Group,
				Kind:  sologatewayv1.RouteOptionGVK.Kind,
			}: routeoptions.NewPlugin(),
		},
	}
}

func (h *ExtensionPluginRegistry) GetExtensionPlugin(gk schema.GroupKind) (ExtensionPlugin, error) {
	p, ok := h.extensionPlugins[schema.GroupKind{
		Group: string(gk.Group),
		Kind:  string(gk.Kind),
	}]
	if !ok {
		return nil, fmt.Errorf("no extenstion support for gk %+v", gk)
	}
	return p, nil
}
