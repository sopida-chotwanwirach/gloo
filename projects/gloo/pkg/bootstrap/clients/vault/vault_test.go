package vault

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = XDescribe("Constructor tests", func() {
	When("We call NewVaultTokenRenewer with default parameters", func() {

		It("Populates the expected default values", func() {
			renewer := NewVaultTokenRenewer(&NewVaultTokenRenewerParams{})

			Expect(renewer.getWatcher).To(Equal(vaultGetWatcher))
			Expect(renewer.leaseIncrement).To(Equal(0))
			Expect(renewer.retryOnNonRenewableSleep).To(Equal(60))

		})

	})

})
