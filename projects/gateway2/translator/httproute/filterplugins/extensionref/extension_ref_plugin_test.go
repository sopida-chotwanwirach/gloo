package extensionref_test

import (
	. "github.com/onsi/ginkgo/v2"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/extensionref"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/registry"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("ExtensionRefPlugin", func() {
	BeforeEach(func() {
	})

	DescribeTable(
		"applying RouteOptions to translated routes",
		func(
			filter gwv1.HTTPRouteFilter,
			expectedRoute *v1.Route,
		) {
			deps := []client.Object{routeOption()}
			queries := testutils.BuildGatewayQueries(deps)
			reg := registry.NewHTTPFilterPluginRegistry(queries)
			getFunc := func(gk schema.GroupKind) (filterplugins.ExtPlugin, error) {
				return reg.GetExtensionPlugin(gk)
			}
			plugin := extensionref.NewPlugin(queries, getFunc)
			filtertests.AssertExpectedRoute(
				plugin,
				filter,
				expectedRoute,
				true,
			)
		},
		Entry(
			"applies fault injecton RouteOptions directly from resource to output route",
			gwv1.HTTPRouteFilter{
				Type: gwv1.HTTPRouteFilterExtensionRef,
				ExtensionRef: &gwv1.LocalObjectReference{
					Group: gwv1.Group(sologatewayv1.RouteOptionGVK.Group),
					Kind:  gwv1.Kind(sologatewayv1.RouteOptionGVK.Kind),
					Name:  "policy",
				},
			},
			&v1.Route{
				Options: &v1.RouteOptions{
					Faults: &faultinjection.RouteFaults{
						Abort: &faultinjection.RouteAbort{
							Percentage: 1,
						},
					},
				},
			},
		),
	)
})

func routeOption() *solokubev1.RouteOption {
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy",
			Namespace: "default",
		},
		Spec: sologatewayv1.RouteOption{
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 1.00,
					},
				},
			},
		},
	}
}
