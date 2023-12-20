package extensionref

// import (
// 	errors "github.com/rotisserie/eris"
// 	"github.com/solo-io/gloo/projects/gateway2/query"
// 	"github.com/solo-io/gloo/projects/gateway2/translator/extensions"
// 	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
// 	"k8s.io/apimachinery/pkg/runtime/schema"
// 	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
// )

// type Plugin struct {
// 	queries  query.GatewayQueries
// 	registry *extensions.ExtensionPluginRegistry
// }

// func NewPlugin(
// 	queries query.GatewayQueries,
// 	registry *extensions.ExtensionPluginRegistry,
// ) *Plugin {
// 	return &Plugin{
// 		queries,
// 		registry,
// 	}
// }

// func (p *Plugin) ApplyFilter(
// 	filter gwv1.HTTPRouteFilter,
// 	outputRoute *v1.Route,
// ) error {
// 	if filter.Type != gwv1.HTTPRouteFilterExtensionRef {
// 		return errors.Errorf("unsupported filter type: %v", filter.Type)
// 	}
// 	if filter.ExtensionRef == nil {
// 		return errors.Errorf("ExtensionRef filter called with nil ExtensionRef field")
// 	}

// 	gk := schema.GroupKind{
// 		Group: string(filter.ExtensionRef.Group),
// 		Kind:  string(filter.ExtensionRef.Kind),
// 	}
// 	plugin, err := p.registry.GetExtensionPlugin(gk)
// 	if err != nil {
// 		return err
// 	}

// 	obj, err := p.queries.GetLocalObjRef(ctx.Ctx, p.queries.ObjToFrom(ctx.Route), *filter.ExtensionRef)
// 	if err != nil {
// 		return err
// 	}

// 	err = plugin.ApplyExtPlugin(ctx.Ctx, obj, outputRoute)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }
