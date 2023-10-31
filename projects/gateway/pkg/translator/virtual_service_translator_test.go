package translator_test

import (
	"context"
	"fmt"
	v12 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	gloodefaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	core2 "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	. "github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
)

const (
	proxyName = "gateway-proxy"
	namespace = gloodefaults.GlooSystem
)

var _ = Describe("Virtual Service Translator", func() {

	// There are many tests for the VirtualServiceTranslator that still live in http_translator_test.go

	var (
		ctx    context.Context
		cancel context.CancelFunc

		translator *VirtualServiceTranslator
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		translator = &VirtualServiceTranslator{
			WarnOnRouteShortCircuiting: false,
		}
	})

	AfterEach(func() {
		cancel()
	})

	// executeTranslation executes the VirtualServiceTranslator on the given snapshot
	// It creates a default Gateway, but it assumes the caller will provide the relevant VirtualServices in the ApiSnapshot
	executeTranslation := func(snapshot *gloov1snap.ApiSnapshot) ([]*gloov1.VirtualHost, reporter.ResourceReports) {
		// We rely on a ParentGateway to select all the VirtualServices that are relevant to this Gateway
		parentGateway := defaults.DefaultGateway(namespace)

		snapshot.Gateways = v1.GatewayList{parentGateway}

		reports := make(reporter.ResourceReports)
		reports.Accept(snapshot.VirtualServices.AsInputResources()...)
		reports.Accept(snapshot.RouteTables.AsInputResources()...)

		virtualHosts := translator.ComputeVirtualHosts(
			NewTranslatorParams(ctx, snapshot, reports),
			parentGateway,
			snapshot.VirtualServices,
			proxyName)

		return virtualHosts, reports
	}

	Context("Using VirtualService DelegateOptions", func() {

		var snapshot *gloov1snap.ApiSnapshot

		BeforeEach(func() {
			optionEastOne := headerManipulationVHOption("east-one")
			optionEastTwo := headerManipulationVHOption("east-two")

			optionWestOne := headerManipulationVHOption("west-one")
			optionWestTwo := authConfigVHOption("west-two")

			vsEast := helpers.NewVirtualServiceBuilder().
				WithName("vs-east").
				WithDomain("east.com").
				WithNamespace(namespace).
				WithDelegateOptionRefs([]*core.ResourceRef{
					optionEastOne.GetMetadata().Ref(),
					optionEastTwo.GetMetadata().Ref(),
				}).
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: 200,
				}).
				Build()

			vsWest := helpers.NewVirtualServiceBuilder().
				WithName("vs-west").
				WithDomain("west.com").
				WithNamespace(namespace).
				WithDelegateOptionRefs([]*core.ResourceRef{
					optionWestOne.GetMetadata().Ref(),
					optionWestTwo.GetMetadata().Ref(),
				}).
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: 200,
				}).
				Build()

			vsSouth := helpers.NewVirtualServiceBuilder().
				WithName("vs-south").
				WithDomain("south.com").
				WithNamespace(namespace).
				WithDelegateOptionRefs([]*core.ResourceRef{
					// none!
				}).
				WithRouteDirectResponseAction("test", &gloov1.DirectResponseAction{
					Status: 200,
				}).
				Build()

			snapshot = &gloov1snap.ApiSnapshot{
				Gateways: v1.GatewayList{},
				VirtualServices: v1.VirtualServiceList{
					vsEast, vsWest, vsSouth,
				},
				VirtualHostOptions: v1.VirtualHostOptionList{
					optionEastOne, optionEastTwo, optionWestOne, optionWestTwo,
				},
			}
		})

		It("delegated options are merged, but not appended", func() {
			virtualHosts, reports := executeTranslation(snapshot)
			Expect(virtualHosts).To(HaveLen(len(snapshot.VirtualServices)))
			Expect(reports.ValidateStrict()).NotTo(HaveOccurred())

			// This demonstrates that the VirtualHostOptions are merged
			// Even though we have 2 VirtualHostOptions, each with 1 RequestHeaderToAdd, those are merged, and we respect the first definition
			// This is how the feature was built, and I would consider it a feature to have the ability to append headers when consolidating options
			// I could see us expanding this delegation API in 2 directions:
			// 	1. Allow a MergePolicy to be applied, which dictates how to merge 2 objects and how inheritance works
			//	2. Allow Options to limit the scope of their delegation, so that they only apply to certain resources. This way delegation has a handshake and both parties have to agree to it
			virtualHostEast := virtualHosts[0]
			Expect(virtualHostEast.GetDomains()).To(ConsistOf("east.com"))
			Expect(virtualHostEast.GetOptions().GetHeaderManipulation().GetRequestHeadersToAdd()).To(HaveLen(1))
			Expect(virtualHostEast.GetOptions().GetHeaderManipulation().GetRequestHeadersToAdd()[0].GetHeader().GetKey()).To(Equal("x-custom-header-east-one"))
			Expect(virtualHostEast.GetOptions().GetExtauth()).To(BeNil(), "no extauth defined on delegated options")

			virtualHostWest := virtualHosts[1]
			Expect(virtualHostWest.GetDomains()).To(ConsistOf("west.com"))
			Expect(virtualHostWest.GetOptions().GetHeaderManipulation().GetRequestHeadersToAdd()).To(HaveLen(1))
			Expect(virtualHostWest.GetOptions().GetHeaderManipulation().GetRequestHeadersToAdd()[0].GetHeader().GetKey()).To(Equal("x-custom-header-west-one"))
			Expect(virtualHostWest.GetOptions().GetExtauth().GetConfigRef().GetName()).To(Equal("west-two"))

			virtualHostSouth := virtualHosts[2]
			Expect(virtualHostSouth.GetDomains()).To(ConsistOf("south.com"))
			Expect(virtualHostSouth.GetOptions().GetHeaderManipulation()).To(BeNil(), "no header manipulation defined")
			Expect(virtualHostSouth.GetOptions().GetExtauth()).To(BeNil(), "no extauth defined")
		})

	})

})

// Utilities for creating VirtualHostOptions
// If these become more widely used, we can move them to t

func headerManipulationVHOption(name string) *v1.VirtualHostOption {
	return &v1.VirtualHostOption{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Options: &gloov1.VirtualHostOptions{
			HeaderManipulation: &headers.HeaderManipulation{
				RequestHeadersToAdd: []*core2.HeaderValueOption{{
					HeaderOption: &core2.HeaderValueOption_Header{
						Header: &core2.HeaderValue{
							Key:   fmt.Sprintf("x-custom-header-%s", name),
							Value: fmt.Sprintf("value-customer-header-%s", name),
						},
					},
				}},
			},
		},
	}
}

func authConfigVHOption(name string) *v1.VirtualHostOption {
	return &v1.VirtualHostOption{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Options: &gloov1.VirtualHostOptions{
			Extauth: &v12.ExtAuthExtension{
				Spec: &v12.ExtAuthExtension_ConfigRef{
					// These tests do not rely on this ConfigRef pointing to a valid resource
					ConfigRef: &core.ResourceRef{
						Name:      name,
						Namespace: namespace,
					},
				},
			},
		},
	}
}
