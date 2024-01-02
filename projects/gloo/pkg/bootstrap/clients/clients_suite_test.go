package clients_test

import (
	"testing"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBootstrapClients(t *testing.T) {
	leakDetector := helpers.DeferredGoroutineLeakDetector(t)
	defer leakDetector()
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Clients Suite")
}

var (
	consulFactory  *services.ConsulFactory
	consulInstance *services.ConsulInstance
)

var _ = BeforeSuite(func() {
	var err error
	consulFactory, err = services.NewConsulFactory()
	Expect(err).NotTo(HaveOccurred())

})

var _ = AfterSuite(func() {
	_ = consulFactory.Clean()
})
