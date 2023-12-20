package filtertests

import (
	"context"
	"log"

	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions"
	"github.com/solo-io/gloo/projects/gateway2/translator/extensions/filterplugins/api"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AssertExpectedRoute(
	plugin api.HTTPFilterPlugin,
	filter gwv1.HTTPRouteFilter,
	expectedRoute *v1.Route,
	logActual bool,
) {
	outputRoute := &v1.Route{
		Options: &v1.RouteOptions{},
	}
	assertExpectedRoute(plugin, filter, outputRoute, expectedRoute, nil, logActual)
}

func AssertExpectedRouteWith(
	plugin api.HTTPFilterPlugin,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
) {
	assertExpectedRoute(plugin, filter, outputRoute, expectedRoute, match, logActual)
}

func assertExpectedRoute(
	plugin api.HTTPFilterPlugin,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
) {
	rtCtx := &extensions.RouteContext{
		Route:    &gwv1.HTTPRoute{},
		Rule:     nil,
		Reporter: nil,
		Match:    match,
	}
	err := plugin.ApplyFilter(
		context.Background(),
		rtCtx,
		filter,
		outputRoute,
	)
	Expect(err).NotTo(HaveOccurred())

	if logActual {
		actualYaml, err := testutils.MarshalYaml(outputRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("actualYaml: \n---\n", string(actualYaml), "\n---\n")
		expectedYaml, err := testutils.MarshalYaml(expectedRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("expectedYaml: \n---\n", string(expectedYaml), "\n---\n")
	}
	Expect(proto.Equal(outputRoute, expectedRoute)).To(BeTrue())
}
