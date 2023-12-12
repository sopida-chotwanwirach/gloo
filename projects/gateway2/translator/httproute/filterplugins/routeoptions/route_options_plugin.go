package routeoptions

import (
	errors "github.com/rotisserie/eris"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct {
	queries query.GatewayQueries
}

func NewPlugin(queries query.GatewayQueries) *Plugin {
	return &Plugin{
		queries,
	}
}

func (p *Plugin) ApplyFilter(
	ctx *filterplugins.RouteContext,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	if filter.Type != gwv1.HTTPRouteFilterExtensionRef {
		return errors.Errorf("unsupported filter type: %v", filter.Type)
	}
	if filter.ExtensionRef == nil {
		return errors.Errorf("RouteOptions ExtensionRef filter called with nil ExtensionRef field")
	}

	// key := types.NamespacedName{
	// 	Namespace: routeNs,
	// 	Name:      string(filter.ExtensionRef.Name),
	// }
	// err := p.client.Get(ctx.Ctx, key, &routeOption)
	obj, err := p.queries.GetLocalObjRef(ctx.Ctx, p.queries.ObjToFrom(ctx.Route), *filter.ExtensionRef)
	if err != nil {
		//TODO: handle not found
		return err
	}
	routeOption := obj.(*solokubev1.RouteOption)

	if routeOption.Spec.Options != nil {
		// set options from RouteOptions resource and clobber any existing options
		// should be revisited if/when we support merging options from e.g. other HTTPRouteFilters
		outputRoute.Options = routeOption.Spec.Options
	}
	return nil
}
