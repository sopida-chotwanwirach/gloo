package filtertests

import (
	"context"
	"log"

	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func AssertExpectedRouteExtPlugin(
	plugin filterplugins.ExtPlugin,
	cfg client.Object,
	expectedRoute *v1.Route,
	logActual bool,
) {
	outputRoute := &v1.Route{
		Options: &v1.RouteOptions{},
	}
	assertExpectedRouteExt(plugin, cfg, outputRoute, expectedRoute, nil, logActual)
}

func AssertExpectedRoute(
	plugin filterplugins.FilterPlugin,
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
	plugin filterplugins.FilterPlugin,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
) {
	assertExpectedRoute(plugin, filter, outputRoute, expectedRoute, match, logActual)
}

func assertExpectedRouteExt(
	plugin filterplugins.ExtPlugin,
	cfg client.Object,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
) {
	ctx := &filterplugins.RouteContext{
		Ctx:      context.TODO(),
		Route:    &gwv1.HTTPRoute{},
		Queries:  nil,
		Rule:     nil,
		Reporter: nil,
		Match:    match,
	}
	err := applyExtPlugin(plugin, ctx, cfg, outputRoute)
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

func assertExpectedRoute(
	plugin filterplugins.FilterPlugin,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
	expectedRoute *v1.Route,
	match *gwv1.HTTPRouteMatch,
	logActual bool,
) {
	ctx := &filterplugins.RouteContext{
		Ctx:      context.TODO(),
		Route:    &gwv1.HTTPRoute{},
		Queries:  nil,
		Rule:     nil,
		Reporter: nil,
		Match:    match,
	}
	err := applyPlugin(plugin, ctx, filter, outputRoute)
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

func applyExtPlugin(
	plugin filterplugins.ExtPlugin,
	ctx *filterplugins.RouteContext,
	cfg client.Object,
	outputRoute *v1.Route,
) error {
	return plugin.ApplyExtPlugin(
		ctx,
		cfg,
		outputRoute,
	)
}

func applyPlugin(
	plugin filterplugins.FilterPlugin,
	ctx *filterplugins.RouteContext,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	return plugin.ApplyFilter(
		ctx,
		filter,
		outputRoute,
	)
}
