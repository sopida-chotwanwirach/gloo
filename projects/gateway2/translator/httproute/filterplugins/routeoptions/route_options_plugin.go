package routeoptions

import (
	"github.com/rotisserie/eris"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyExtPlugin(
	ctx *filterplugins.RouteContext,
	cfg client.Object,
	outputRoute *v1.Route,
) error {
	routeOption, ok := cfg.(*solokubev1.RouteOption)
	if !ok {
		return eris.Errorf("cfg object passed to RouteOptionsPlugin is not a RouteOption type")
	}

	if routeOption.Spec.Options != nil {
		// set options from RouteOptions resource and clobber any existing options
		// should be revisited if/when we support merging options from e.g. other HTTPRouteFilters
		outputRoute.Options = routeOption.Spec.Options
	}
	return nil
}
