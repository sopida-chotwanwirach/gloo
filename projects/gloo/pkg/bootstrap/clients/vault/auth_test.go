package vault_test

import (
    "context"
    "github.com/golang/mock/gomock"
    "github.com/hashicorp/vault/api"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/onsi/gomega/types"
    "github.com/rotisserie/eris"
    v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
    "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
    "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
)

var _ = FDescribe("AuthMethod", func() {

    var (
        ctx    context.Context
        cancel context.CancelFunc

        authMethod api.AuthMethod
    )

    BeforeEach(func() {
        ctx, cancel = context.WithCancel(context.Background())

        // The tests below will be responsible for assigning this variable
        // We re-set it here, just to be safe
        authMethod = nil
    })

    AfterEach(func() {
        cancel()
    })

    Context("StaticTokenAuthMethod", func() {
        // These tests validate the behavior of the staticTokenAuthMethod implementation of the vault.AuthMethod interface

        When("token is empty", func() {

            BeforeEach(func() {
                authMethod = vault.NewStaticTokenAuthMethod("")
            })

            It("should return an error", func() {
                _, err := authMethod.Login(ctx, nil)
                Expect(err).To(HaveOccurred())

                // todo - assert metrics increased
            })

        })

        When("token is not empty", func() {

            BeforeEach(func() {
                authMethod = vault.NewStaticTokenAuthMethod("placeholder")
            })

            It("should return a vault.Secret", func() {
                secret, err := authMethod.Login(ctx, nil)
                Expect(err).NotTo(HaveOccurred())
                Expect(secret).NotTo(BeNil())

                // todo - assert secret contains relevant dataa

                // todo - assert metrics increased
            })

        })

    })

    Context("RetryableAuthMethod", func() {
        // These tests validate the behavior of the retryableAuthMethod implementation of the vault.AuthMethod interface

        When("internal auth method returns an error", func() {

            BeforeEach(func() {

                ctrl := gomock.NewController(GinkgoT())
                internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
                internalAuthMethod.EXPECT().Login(ctx, nil).Return(nil, eris.New("error")).AnyTimes()

                authMethod = vault.NewRetryableAuthMethod(internalAuthMethod)
            })

            It("should return an error", func() {
                _, err := authMethod.Login(ctx, nil)
                Expect(err).To(HaveOccurred())

                // todo - assert metrics increased

            })

        })

    })

    Context("NewAuthMethodForSettings", func() {
        // These tests validate that the constructor maps the Gloo Settings into the appropriate vault.AuthMethod interface
        // it does not test the underlying implementations, as those are handled in the above tests

        DescribeTable("should error on invalid inputs",
            func(vaultSettings *v1.Settings_VaultSecrets, expectedError types.GomegaMatcher) {
                _, err := vault.NewAuthMethodForSettings(ctx, vaultSettings)
                Expect(err).To(expectedError)
            },
            Entry("partial accessKey / secretAccessKey", &v1.Settings_VaultSecrets{
                AuthMethod: &v1.Settings_VaultSecrets_Aws{
                    Aws: &v1.Settings_VaultAwsAuth{
                        AccessKeyId:     "access-key-id",
                        SecretAccessKey: "",
                    },
                },
            }, HaveOccurred()), // this should be improved to be more explicit
        )

        DescribeTable("should return the correct auth method",
            func(vaultSettings *v1.Settings_VaultSecrets, expectedAuthMethod types.GomegaMatcher) {
                authMethod, err := vault.NewAuthMethodForSettings(ctx, vaultSettings)
                Expect(err).NotTo(HaveOccurred())
                Expect(authMethod).To(expectedAuthMethod)
            },
            Entry("nil", nil, Not(BeNil())), // this should be improved
        )

    })
})
