package istio_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	testutils2 "github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/go-utils/testutils/exec"

	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/gloo/test/kube2e/helper"
	skhelpers "github.com/solo-io/solo-kit/test/helpers"
)

const (
	gatewayProxy     = gatewaydefaults.GatewayProxyName
	gatewayPort      = int(80)
	installNamespace = defaults.GlooSystem
	httpbinNamespace = "httpbin-ns"
)

func TestIstio(t *testing.T) {
	helpers.RegisterGlooDebugLogPrintHandlerAndClearLogs()
	skhelpers.RegisterCommonFailHandlers()
	skhelpers.SetupLog()
	_ = os.Remove(cliutil.GetLogsPath())
	RunSpecs(t, "Istio Suite")
}

var (
	testHelper *helper.SoloTestHelper
	ctx        context.Context
	cancel     context.CancelFunc

	cwd string

	resourceClientSet *kube2e.KubeResourceClientSet
)

var _ = BeforeSuite(func() {
	var err error

	ctx, cancel = context.WithCancel(context.Background())

	cwd, err = os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "working dir could not be retrieved")

	testHelper, err = kube2e.GetTestHelper(ctx, installNamespace)
	Expect(err).NotTo(HaveOccurred())

	skhelpers.RegisterPreFailHandler(helpers.StandardGlooDumpOnFail(GinkgoWriter, testHelper.InstallNamespace))

	err = testutils.Kubectl("create", "ns", testHelper.InstallNamespace)
	Expect(err).NotTo(HaveOccurred())

	if !testutils2.ShouldSkipInstall() {
		installGloo()
	}

	// add httpbin app in its own namespace, labeled for Istio injection
	err = testutils.Kubectl("create", "ns", httpbinNamespace)
	Expect(err).NotTo(HaveOccurred())

	err = testutils.Kubectl("label", "namespace", httpbinNamespace, "istio-injection=enabled")
	Expect(err).NotTo(HaveOccurred())

	err = testutils.Kubectl("apply", "-n", httpbinNamespace, "-f", filepath.Join(cwd, "artifacts", "httpbin.yaml"))
	Expect(err).NotTo(HaveOccurred())

	// delete test-runner Service, as the tests create and manage their own
	err = testutils.Kubectl("delete", "service", helper.TestrunnerName, "-n", installNamespace)
	Expect(err).NotTo(HaveOccurred())
	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "service", helper.TestrunnerName, "-n", installNamespace)
	}, "60s", "1s").Should(HaveOccurred())

	expectIstioInjected()

	resourceClientSet, err = kube2e.NewDefaultKubeResourceClientSet(ctx)
	Expect(err).NotTo(HaveOccurred(), "can create kube resource client set")
})

var _ = AfterSuite(func() {
	if testutils2.ShouldTearDown() {
		uninstallGloo()

		err := testutils.Kubectl("delete", "namespace", httpbinNamespace)
		Expect(err).NotTo(HaveOccurred())
	}

	cancel()
})

func installGloo() {
	helmValuesFile := filepath.Join(cwd, "artifacts", "helm.yaml")

	// Install Gloo
	// this helper function also applies the testrunner pod and service
	err := testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", helmValuesFile))
	Expect(err).NotTo(HaveOccurred())

	// Check that everything is OK
	kube2e.GlooctlCheckEventuallyHealthy(1, testHelper, "90s")

	// Ensure gloo reaches valid state and doesn't continually resync
	// we can consider doing the same for leaking go-routines after resyncs
	kube2e.EventuallyReachesConsistentState(testHelper.InstallNamespace)
}

func uninstallGloo() {
	err := testHelper.UninstallGlooAll()
	Expect(err).NotTo(HaveOccurred())

	// glooctl should delete the namespace. we do it again just in case it failed
	// ignore errors
	_ = testutils.Kubectl("delete", "namespace", testHelper.InstallNamespace)

	EventuallyWithOffset(1, func() error {
		return testutils.Kubectl("get", "namespace", testHelper.InstallNamespace)
	}, "60s", "1s").Should(HaveOccurred())
}

// expects gateway-proxy and httpbin to have the istio-proxy sidecar, testrunner should not
func expectIstioInjected() {
	// Check for istio-proxy sidecar
	istioContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", "-l", "gloo=gateway-proxy", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on gateway-proxy due to IstioSDS being enabled")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	istioContainer, err = exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", helper.TestrunnerName, "-o", `jsonpath='{.spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("''"), "istio-proxy container should not be present on the testrunner")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	istioContainer, err = exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", httpbinNamespace, "pods", "-l", "app=httpbin", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "istio-proxy")].name}'`)
	ExpectWithOffset(1, istioContainer).To(Equal("'istio-proxy'"), "istio-proxy container should be present on the httpbin pod after injection")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	// Check for sds container
	sdsContainer, err := exec.RunCommandOutput(testHelper.RootDir, false, "kubectl", "get", "-n", testHelper.InstallNamespace, "pods", "-l", "gloo=gateway-proxy", "-o", `jsonpath='{.items[*].spec.containers[?(@.name == "sds")].name}'`)
	ExpectWithOffset(1, sdsContainer).To(Equal("'sds'"), "sds container should be present on gateway-proxy due to IstioSDS being enabled")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}
