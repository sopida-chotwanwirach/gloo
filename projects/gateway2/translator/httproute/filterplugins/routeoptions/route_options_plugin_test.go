package routeoptions_test

import (
	. "github.com/onsi/ginkgo/v2"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwscheme "github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/routeoptions"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("RouteOptionsPlugin", func() {
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
			cfg *solokubev1.RouteOption,
			expectedRoute *v1.Route,
		) {
			plugin := routeoptions.NewPlugin()
			filtertests.AssertExpectedRouteExtPlugin(
				plugin,
				cfg,
				expectedRoute,
				true,
			)
		},
		Entry(
			"applies fault injecton RouteOptions directly from resource to output route",
			&solokubev1.RouteOption{
				Spec: sologatewayv1.RouteOption{
					Options: &v1.RouteOptions{
						Faults: &faultinjection.RouteFaults{
							Abort: &faultinjection.RouteAbort{
								Percentage: 1.00,
							},
						},
					},
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
