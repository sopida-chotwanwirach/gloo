package extensiontests

import (
	"context"
	"log"

	"github.com/solo-io/gloo/projects/gateway2/translator/extensions"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func AssertExpectedRouteExtPlugin(
	plugin extensions.ExtensionPlugin,
	cfg client.Object,
	expectedRoute *v1.Route,
	shouldLog bool,
) {
	ctx := context.Background()

	outputRoute := &v1.Route{
		Options: &v1.RouteOptions{},
	}

	err := plugin.ApplyExtPlugin(
		ctx,
		cfg,
		outputRoute,
	)
	Expect(err).NotTo(HaveOccurred())

	logYaml(shouldLog, expectedRoute, outputRoute)

	Expect(proto.Equal(outputRoute, expectedRoute)).To(BeTrue())
}

func logYaml(shouldLog bool, expectedRoute *v1.Route, actualRoute *v1.Route) {
	if shouldLog {
		actualYaml, err := testutils.MarshalYaml(actualRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("actualYaml: \n---\n", string(actualYaml), "\n---\n")
		expectedYaml, err := testutils.MarshalYaml(expectedRoute)
		Expect(err).NotTo(HaveOccurred())
		log.Print("expectedYaml: \n---\n", string(expectedYaml), "\n---\n")
	}
}
