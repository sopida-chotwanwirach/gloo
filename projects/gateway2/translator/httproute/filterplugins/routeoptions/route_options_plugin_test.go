package routeoptions

import (
	. "github.com/onsi/ginkgo/v2"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwscheme "github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("RouteOptionsPlugin", func() {
	// var (
	// 	scheme     *runtime.Scheme
	// 	builder    *fake.ClientBuilder
	// 	fakeClient client.Client
	// )

	BeforeEach(func() {
		scheme := gwscheme.NewScheme()
		builder := fake.NewClientBuilder().WithScheme(scheme)
		query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
	})

	DescribeTable(
		"applying RouteOptions to translated routes",
		func(
			filter gwv1.HTTPRouteFilter,
			expectedRoute *v1.Route,
		) {
			// fakeClient = builder.WithObjects(routeOption()).Build()
			// queries := query.NewData(fakeClient, scheme)

			deps := []client.Object{routeOption()}
			queries := testutils.BuildGatewayQueries(deps)
			plugin := NewPlugin(queries)
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
