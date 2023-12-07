package e2e_test

import (
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	bootstrap "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients"
	"github.com/solo-io/gloo/test/e2e"
	"github.com/solo-io/gloo/test/ginkgo/decorators"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	// These tests run using the following AWS ARN for the Vault Role
	// If you want to run these tests locally, ensure that your local AWS credentials match,
	// or use another role
	// https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-profiles.html
	// Please note that although this is used as a "role" in vault (the value is written to "auth/aws/role/vault-role")
	// it is actually an aws user so if running locally *user* and not the role that gets created during manual setup
	vaultAwsRole = "arn:aws:iam::802411188784:user/gloo-edge-e2e-user"
	//vaultAwsRole   = "arn:aws:iam::802411188784:user/sheidkamp"
	vaultAwsRegion = "us-east-1"

	vaultRole = "vault-role"
)

var _ = Describe("Vault Secret Store (AWS Auth)", decorators.Vault, func() {

	var (
		testContext         *e2e.TestContextWithVault
		vaultSecretSettings *gloov1.Settings_VaultSecrets
		oauthSecret         *gloov1.Secret
	)

	BeforeEach(func() {
		testContext = testContextFactory.NewTestContextWithVault(testutils.AwsCredentials())
		testContext.BeforeEach()

		oauthSecret = &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "oauth-secret",
				Namespace: writeNamespace,
			},
			Kind: &gloov1.Secret_Oauth{
				Oauth: &v1.OauthSecret{
					ClientSecret: "test",
				},
			},
		}

		testContext.ResourcesToCreate().Secrets = gloov1.SecretList{
			oauthSecret,
		}
	})

	AfterEach(func() {
		testContext.AfterEach()
	})

	JustBeforeEach(func() {
		testContext.SetRunSettings(&gloov1.Settings{
			SecretSource: &gloov1.Settings_VaultSecretSource{
				VaultSecretSource: vaultSecretSettings,
			},
		})
		testContext.RunVault()

		// We need to turn on Vault AWS Auth after it has started running
		err := testContext.VaultInstance().EnableAWSCredentialsAuthMethod(vaultSecretSettings, vaultAwsRole)
		Expect(err).NotTo(HaveOccurred())

		testContext.JustBeforeEach()
	})

	JustAfterEach(func() {
		testContext.JustAfterEach()
	})

	Context("Vault Credentials", func() {
		BeforeEach(func() {
			localAwsCredentials := credentials.NewSharedCredentials("", "")
			v, err := localAwsCredentials.Get()
			Expect(err).NotTo(HaveOccurred(), "can load AWS shared credentials")

			vaultSecretSettings = &gloov1.Settings_VaultSecrets{
				Address: testContext.VaultInstance().Address(),
				AuthMethod: &gloov1.Settings_VaultSecrets_Aws{
					Aws: &gloov1.Settings_VaultAwsAuth{
						VaultRole:       vaultRole,
						Region:          vaultAwsRegion,
						AccessKeyId:     v.AccessKeyID,
						SecretAccessKey: v.SecretAccessKey,
					},
				},
				PathPrefix: bootstrap.DefaultPathPrefix,
				RootKey:    bootstrap.DefaultRootKey,
			}
		})

		It("can read secret using resource client", func() {
			var (
				secret *gloov1.Secret
				err    error
			)

			Eventually(func(g Gomega) {
				secret, err = testContext.TestClients().SecretClient.Read(
					oauthSecret.GetMetadata().GetNamespace(),
					oauthSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("test"))
			}, "5s", ".5s").Should(Succeed())

			// Sleep and try again
			time.Sleep(20 * time.Second)

			// We are setting the ttl of the secret lease to 10 seconds, so this would fail without the goroutine which renews the lease.
			// To see this fail, comment out the call to the 'renewToken' goroutine in pkg/bootstrap/clients/vault.go
			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					oauthSecret.GetMetadata().GetNamespace(),
					oauthSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("test"))
			}, "5s", ".5s").Should(Succeed())
		})

		It("can pick up new secrets created by vault client ", func() {
			newSecret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "new-secret",
					Namespace: writeNamespace,
				},
				Kind: &gloov1.Secret_Oauth{
					Oauth: &v1.OauthSecret{
						ClientSecret: "new-secret",
					},
				},
			}

			err := testContext.VaultInstance().WriteSecret(newSecret)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				secret, err := testContext.TestClients().SecretClient.Read(
					newSecret.GetMetadata().GetNamespace(),
					newSecret.GetMetadata().GetName(),
					clients.ReadOpts{
						Ctx: testContext.Ctx(),
					})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secret.GetOauth().GetClientSecret()).To(Equal("new-secret"))
			}, "5s", ".5s").Should(Succeed())
		})

	})

})
