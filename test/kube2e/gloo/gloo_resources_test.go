package gloo_test

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes/serviceconverter"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"

	"time"

	kubev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("GlooResourcesTest", func() {

	var (
		testRunnerDestination *gloov1.Destination
		testRunnerVs          *gatewayv1.VirtualService

		glooResources *gloosnapshot.ApiSnapshot
	)

	BeforeEach(func() {
		// Create a VirtualService routing directly to the testrunner kubernetes service
		testRunnerDestination = &gloov1.Destination{
			DestinationType: &gloov1.Destination_Kube{
				Kube: &gloov1.KubernetesServiceDestination{
					Ref: &core.ResourceRef{
						Namespace: testHelper.InstallNamespace,
						Name:      helper.TestrunnerName,
					},
					Port: uint32(helper.TestRunnerPort),
				},
			},
		}
		testRunnerVs = helpers.NewVirtualServiceBuilder().
			WithName(helper.TestrunnerName).
			WithNamespace(testHelper.InstallNamespace).
			WithLabel(kube2e.UniqueTestResourceLabel, uuid.New().String()).
			WithDomain(helper.TestrunnerName).
			WithRoutePrefixMatcher(helper.TestrunnerName, "/").
			WithRouteActionToSingleDestination(helper.TestrunnerName, testRunnerDestination).
			Build()

		// The set of resources that these tests will generate
		glooResources = &gloosnapshot.ApiSnapshot{
			VirtualServices: gatewayv1.VirtualServiceList{
				// many tests route to the TestRunner Service so it makes sense to just
				// always create it
				// the other benefit is this ensures that all tests start with a valid Proxy CR
				testRunnerVs,
			},
		}
	})

	JustBeforeEach(func() {
		err := snapshotWriter.WriteSnapshot(glooResources, clients.WriteOpts{
			Ctx:               ctx,
			OverwriteExisting: false,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	JustAfterEach(func() {
		err := snapshotWriter.DeleteSnapshot(glooResources, clients.DeleteOpts{
			Ctx:            ctx,
			IgnoreNotExist: true,
		})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("rotating secrets on upstream sslConfig", func() {

		var (
			tlsSecret *kubev1.Secret
		)

		BeforeEach(func() {
			// Gen crt and key for python server to use, doens't matter that it will be discarded
			// because validation is off by default
			crt, crtKey := helpers.GetCerts(helpers.Params{
				Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
				IsCA:  true,
			})
			err := testHelper.TestUpstreamServer.DeployServerTls(time.Second*600, []byte(crt), []byte(crtKey))
			Expect(err).NotTo(HaveOccurred())

			tlsSecret = helpers.GetKubeSecret("secret", testHelper.InstallNamespace)

			_, err = resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Create(ctx, tlsSecret, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			upstreamSslConfig := &ssl.UpstreamSslConfig{
				SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
					SecretRef: &core.ResourceRef{
						Name:      tlsSecret.GetName(),
						Namespace: tlsSecret.GetNamespace(),
					},
				},
			}

			Eventually(func(g Gomega) {
				testRunnerService, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, helper.TestrunnerName, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())

				setAnnotations(testRunnerService, map[string]string{
					serviceconverter.DeepMergeAnnotationPrefix: "true",
					serviceconverter.GlooAnnotationPrefix: fmt.Sprintf(`{
							"sslConfig": {
								"secretRef": {
									"name": "%s",
									"namespace":  "%s"
								}
							}
						}`, tlsSecret.GetName(), tlsSecret.GetNamespace()),
				})
				_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, testRunnerService, metav1.UpdateOptions{})
				g.Expect(err).NotTo(HaveOccurred())
			}, "30s", "1s").Should(Succeed(), "annotate the kube service, so that discovery applies the ssl configuration to the generated upstream")

			Eventually(func(g Gomega) {
				usName := kubernetes.UpstreamName(testHelper.InstallNamespace, helper.TestrunnerName, helper.TestRunnerPort)
				testRunnerUs, err := resourceClientset.UpstreamClient().Read(testHelper.InstallNamespace, usName, clients.ReadOpts{Ctx: ctx})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(testRunnerUs.GetSslConfig()).To(matchers.MatchProto(upstreamSslConfig))
			}, "30s", "1s").Should(Succeed(), "the kube upstream should eventually contain the ssl configuration")

		})

		AfterEach(func() {
			err := resourceClientset.KubeClients().CoreV1().Secrets(testHelper.InstallNamespace).Delete(ctx, tlsSecret.GetName(), metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				testRunnerService, err := resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Get(ctx, helper.TestrunnerName, metav1.GetOptions{})
				g.Expect(err).NotTo(HaveOccurred())

				setAnnotations(testRunnerService, map[string]string{
					serviceconverter.DeepMergeAnnotationPrefix: "",
					serviceconverter.GlooAnnotationPrefix:      "",
				})
				_, err = resourceClientset.KubeClients().CoreV1().Services(testHelper.InstallNamespace).Update(ctx, testRunnerService, metav1.UpdateOptions{})
				g.Expect(err).NotTo(HaveOccurred())
			}, "30s", "1s").Should(Succeed(), "remove the ssl config annotation from the test runner service")

		})

		It("Should be able to rotate a secret referenced on a sslConfig on a kube upstream", func() {
			// During the delivery of https://github.com/solo-io/gloo/pull/9007, we learned that this test does not work.
			// At the moment, we ignore this test in CI, but we intend to fix it in the near future.
			// https://github.com/solo-io/gloo/issues/6686
			// Skip("Auto-skipping broken test")

			// this test will call the upstream multiple times and confirm that the response from the upstream is not `no healthy upstream`
			// the sslConfig should be rotated and given time to rotate in the upstream. There is a 15 second delay, that sometimes takes longer,
			// for the upstream to fail. The fail happens randomly so the curl must happen multiple times.

			// 22 seconds between rotation with the offset added as well
			secondsForCurling := 22 * time.Second
			// offset to add for longer curls, this might make the number of times performed increase
			offset := 2 * time.Second
			// time given for a single curl
			timeForCurling := 5 * time.Second
			// eventually the `no healthy upstream` will occur
			timesToPerform := time.Duration(10)

			timeInBetweenRotation := secondsForCurling + timeForCurling + offset

			Consistently(func(g Gomega) {
				By("Generate new CaCrt and PrivateKey")
				crt, crtKey := helpers.GetCerts(helpers.Params{
					Hosts: "gateway-proxy,knative-proxy,ingress-proxy",
					IsCA:  true,
				})

				By("Update the kube secret with the new values")
				tlsSecret.Data = map[string][]byte{
					kubev1.TLSCertKey:       []byte(crt),
					kubev1.TLSPrivateKeyKey: []byte(crtKey),
				}
				_, err := resourceClientset.KubeClients().CoreV1().Secrets(tlsSecret.GetNamespace()).Update(ctx, tlsSecret, metav1.UpdateOptions{})
				Expect(err).NotTo(HaveOccurred())

				By("Eventually can curl the endpoint")
				testHelper.CurlEventuallyShouldRespond(helper.CurlOpts{
					Protocol:          "http",
					Path:              "/",
					Method:            "GET",
					Host:              helper.TestrunnerName,
					Service:           defaults.GatewayProxyName,
					Port:              80,
					ConnectionTimeout: 1,
					WithoutStats:      true,
					Verbose:           true,
					SelfSigned:        true,
				}, kube2e.GetSimpleTestRunnerHttpResponse(), 0, 60*time.Second, 1*time.Second)

			}, timeInBetweenRotation*timesToPerform, timeInBetweenRotation).Should(Succeed())
		})
	})
})

func setAnnotations(service *kubev1.Service, annotations map[string]string) {
	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}

	for k, v := range annotations {
		if v == "" {
			// If the value is empty, delete the annotation
			delete(service.Annotations, k)
		} else {
			// If the value is non-empty, set the annotation
			service.Annotations[k] = v
		}

	}
}
