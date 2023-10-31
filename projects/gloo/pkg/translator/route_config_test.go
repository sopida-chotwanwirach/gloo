package translator_test

import (
    "context"
    "fmt"
    routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
    envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
    "github.com/envoyproxy/go-control-plane/pkg/wellknown"
    "github.com/golang/protobuf/ptypes"
    v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
    extauth "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
    gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
    "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
    "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
    "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
    "github.com/solo-io/gloo/projects/gloo/pkg/plugins"
    "github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
    "github.com/solo-io/gloo/projects/gloo/pkg/utils/validation"
    core2 "github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
    "github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
    "strconv"
    "strings"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("Route Configs", func() {

    DescribeTable("validate route path", func(path string, expectedValue bool) {
        if expectedValue {
            Expect(translator.ValidateRoutePath(path)).ToNot(HaveOccurred())
        } else {
            Expect(translator.ValidateRoutePath(path)).To(HaveOccurred())
        }
    },
        Entry("Hex", "%af", true),
        Entry("Hex Camel", "%Af", true),
        Entry("Hex num", "%00", true),
        Entry("Hex double", "%11", true),
        Entry("Hex with valid", "%af801&*", true),
        Entry("valid with hex", "801&*%af", true),
        Entry("valid with hex and valid", "801&*%af719$@!", true),
        Entry("Hex single", "%0", false),
        Entry("unicode chars", "ƒ©", false),
        Entry("unicode chars", "¥¨˚∫", false),
        Entry("//", "hello/something//", false),
        Entry("/./", "hello/something/./", false),
        Entry("/../", "hello/something/../", false),
        Entry("hex slash upper", "hello/something%2F", false),
        Entry("hex slash lower", "hello/something%2f", false),
        Entry("hash", "hello/something#", false),
        Entry("/..", "hello/../something", false),
        Entry("/.", "hello/./something", false),
    )

    It("Should validate all separate characters", func() {
        // must allow all "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
        // https://www.rfc-editor.org/rfc/rfc3986
        // unreserved
        // alpha Upper and Lower
        for i := 'a'; i <= 'z'; i++ {
            Expect(translator.ValidateRoutePath(string(i))).ToNot(HaveOccurred())
            Expect(translator.ValidateRoutePath(strings.ToUpper(string(i)))).ToNot(HaveOccurred())
        }
        // digit
        for i := 0; i < 10; i++ {
            Expect(translator.ValidateRoutePath(strconv.Itoa(i))).ToNot(HaveOccurred())
        }
        unreservedChars := "-._~"
        for _, c := range unreservedChars {
            Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
        }
        // sub-delims
        subDelims := "!$&'()*+,;="
        Expect(len(subDelims)).To(Equal(11))
        for _, c := range subDelims {
            Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
        }
        // pchar
        pchar := ":@"
        for _, c := range pchar {
            Expect(translator.ValidateRoutePath(string(c))).ToNot(HaveOccurred())
        }
        // invalid characters
        invalid := "<>?\\|[]{}\"^%#"
        for _, c := range invalid {
            Expect(translator.ValidateRoutePath(string(c))).To(HaveOccurred())
        }
    })

    DescribeTable("path rewrites", func(s string, pass bool) {
        err := translator.ValidatePrefixRewrite(s)
        if pass {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
        Entry("allow query parameters", "some/site?a=data&b=location&c=searchterm", true),
        Entry("allow fragments", "some/site#framgentedinfo", true),
        Entry("invalid", "some/site<hello", false),
        Entry("invalid", "some/site{hello", false),
        Entry("invalid", "some/site}hello", false),
        Entry("invalid", "some/site[hello", false),
    )

    FContext("httpRouteConfigurationTranslator", func() {

        const (
            routeConfigName = "route-config"
            namespace       = defaults.GlooSystem
        )

        var (
            ctx    context.Context
            cancel context.CancelFunc
        )

        BeforeEach(func() {
            ctx, cancel = context.WithCancel(context.Background())
        })

        AfterEach(func() {
            cancel()
        })

        executeTranslator := func(snapshot *gloov1snap.ApiSnapshot) []*routev3.RouteConfiguration {
            if len(snapshot.Proxies) > 1 {
                panic("Test assumes only 1 Proxy provided")
            }
            proxy := snapshot.Proxies[0]
            proxyReport := validation.MakeReport(proxy)

            if len(proxy.GetListeners()) > 1 {
                panic("Test assumes only 1 Listener provided")
            }
            listener := proxy.GetListeners()[0]

            if listener.GetHttpListener() == nil {
                panic("Test assumes HttpListener provided")
            }
            httpListener := listener.GetHttpListener()

            pluginRegistry := registry.GetPluginRegistryFactory(bootstrap.Opts{})(ctx)

            // Plugins rely on Init, to properly be initialized
            for _, plugin := range pluginRegistry.GetPlugins() {
                plugin.Init(plugins.InitParams{})
            }

            routeConfigTranslator := translator.NewHttpTranslator(translator.HttpTranslatorParams{
                PluginRegistry:           pluginRegistry,
                Proxy:                    proxy,
                ParentListener:           listener,
                Listener:                 httpListener,
                Report:                   proxyReport.GetListenerReports()[0].GetHttpListenerReport(),
                ParentReport:             proxyReport.GetListenerReports()[0],
                RouteConfigName:          routeConfigName,
                RequireTlsOnVirtualHosts: false,
            })

            return routeConfigTranslator.ComputeRouteConfiguration(plugins.Params{
                Ctx:      ctx,
                Snapshot: snapshot,
            })
        }

        It("should not merge virtual hosts with different auth options", func() {
            vhostEast := &v1.VirtualHost{
                Name:    "east",
                Domains: []string{"east.com"},
                Routes: []*v1.Route{{
                    Action: &v1.Route_DirectResponseAction{
                        DirectResponseAction: &v1.DirectResponseAction{
                            Status: 200,
                        },
                    },
                }},
                Options: &v1.VirtualHostOptions{
                    HeaderManipulation: &headers.HeaderManipulation{
                        RequestHeadersToAdd: []*core2.HeaderValueOption{{
                            HeaderOption: &core2.HeaderValueOption_Header{
                                Header: &core2.HeaderValue{
                                    Key:   fmt.Sprintf("x-custom-header-%s", "east"),
                                    Value: fmt.Sprintf("value-customer-header-%s", "east"),
                                },
                            },
                        }},
                    },
                },
            }
            vhostWest := &v1.VirtualHost{
                Name:    "west",
                Domains: []string{"west.com"},
                Routes: []*v1.Route{{
                    Action: &v1.Route_DirectResponseAction{
                        DirectResponseAction: &v1.DirectResponseAction{
                            Status: 200,
                        },
                    },
                }},
                Options: &v1.VirtualHostOptions{
                    Extauth: &extauth.ExtAuthExtension{
                        Spec: &extauth.ExtAuthExtension_CustomAuth{
                            CustomAuth: &extauth.CustomAuth{
                                ContextExtensions: map[string]string{
                                    "ext-authz-header": "ext-authz-header-value",
                                },
                            },
                        },
                    },
                    HeaderManipulation: &headers.HeaderManipulation{
                        RequestHeadersToAdd: []*core2.HeaderValueOption{{
                            HeaderOption: &core2.HeaderValueOption_Header{
                                Header: &core2.HeaderValue{
                                    Key:   fmt.Sprintf("x-custom-header-%s", "west"),
                                    Value: fmt.Sprintf("value-customer-header-%s", "west"),
                                },
                            },
                        }},
                    },
                },
            }

            httpListenerOptions := &v1.HttpListenerOptions{
                // This dictates that the ExtAuth filter should be enabled on the FilterChain
                Extauth: &extauth.Settings{
                    ExtauthzServerRef: &core.ResourceRef{
                        Name:      "extauthz-server",
                        Namespace: namespace,
                    },
                },
            }

            snapshot := &gloov1snap.ApiSnapshot{
                Proxies: []*v1.Proxy{{
                    Listeners: []*v1.Listener{{
                        ListenerType: &v1.Listener_HttpListener{
                            HttpListener: &v1.HttpListener{
                                VirtualHosts: []*v1.VirtualHost{
                                    vhostEast, vhostWest,
                                },
                                Options: httpListenerOptions,
                            },
                        },
                    }},
                }},
                Upstreams: v1.UpstreamList{{
                    Metadata: &core.Metadata{
                        Name:      httpListenerOptions.GetExtauth().GetExtauthzServerRef().GetName(),
                        Namespace: httpListenerOptions.GetExtauth().GetExtauthzServerRef().GetNamespace(),
                    },
                    UpstreamType: &v1.Upstream_Static{},
                }},
            }

            routeConfigList := executeTranslator(snapshot)
            Expect(routeConfigList).To(HaveLen(1))

            routeConfig := routeConfigList[0]
            Expect(routeConfig.GetName()).To(Equal(routeConfigName))

            By("Asserting properties of VirtualHost East", func() {
                envoyVhostEast := routeConfig.GetVirtualHosts()[0]
                Expect(envoyVhostEast.GetName()).To(Equal(vhostEast.Name))

                By("ExtAuthPerRoute", func() {
                    var extAuthConfig envoyauth.ExtAuthzPerRoute
                    err := ptypes.UnmarshalAny(envoyVhostEast.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization], &extAuthConfig)
                    Expect(err).NotTo(HaveOccurred())
                    Expect(extAuthConfig.GetDisabled()).To(BeTrue(), "extauth should be disabled")
                })

                By("HeaderManipulation", func() {
                    Expect(envoyVhostEast.GetRequestHeadersToAdd()).To(HaveLen(1))
                    Expect(envoyVhostEast.GetRequestHeadersToAdd()[0].GetHeader().GetKey()).To(Equal("x-custom-header-east"))
                })
            })

            By("Asserting properties of VirtualHost West", func() {
                envoyVhostWest := routeConfig.GetVirtualHosts()[1]
                Expect(envoyVhostWest.GetName()).To(Equal(vhostWest.Name))

                By("ExtAuthPerRoute", func() {
                    var extAuthConfig envoyauth.ExtAuthzPerRoute
                    err := ptypes.UnmarshalAny(envoyVhostWest.GetTypedPerFilterConfig()[wellknown.HTTPExternalAuthorization], &extAuthConfig)
                    Expect(err).NotTo(HaveOccurred())
                    Expect(extAuthConfig.GetCheckSettings().GetContextExtensions()).To(
                        HaveKeyWithValue("ext-authz-header", "ext-authz-header-value"))
                    Expect(extAuthConfig.GetDisabled()).To(BeFalse(), "extauth should NOT be disabled")
                })

                By("HeaderManipulation", func() {
                    Expect(envoyVhostWest.GetRequestHeadersToAdd()).To(HaveLen(1))
                    Expect(envoyVhostWest.GetRequestHeadersToAdd()[0].GetHeader().GetKey()).To(Equal("x-custom-header-west"))
                })
            })

        })
    })
})
