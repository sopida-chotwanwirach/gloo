package vault_test

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	vaultapi "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	mock_vault "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"

	"github.com/solo-io/gloo/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

type NoOpRenewal struct{}

func (*NoOpRenewal) StartRenewal(ctx context.Context, client *vaultapi.Client, secret *vaultapi.Secret) error {
	return nil
}

var _ = Describe("ClientAuth", func() {

	var (
		ctx    context.Context
		cancel context.CancelFunc
		client *vaultapi.Client

		clientAuth ClientAuth
		ctrl       *gomock.Controller

		mockTokenRenewer *mock_vault.MockTokenRenewer
		//secret     *vaultapi.Secret
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// The tests below will be responsible for assigning this variable
		// We re-set it here, just to be safe
		clientAuth = nil
		//client = nil

		// We should not have any metrics set before running the tests
		// This ensures that we are no leaking metrics between tests
		resetViews()
	})

	JustBeforeEach(func() {
		Expect(clientAuth).NotTo(BeNil())
	})

	AfterEach(func() {
		cancel()
		// Give time for all go rountines to exit
		// TODO - replace with eventually of some sort?
		//time.Sleep(3 * time.Second)
	})

	Context("Access Token Auth", func() {
		// These tests validate the behavior of the StaticTokenAuth implementation of the ClientAuth interface

		When("token is empty", func() {

			BeforeEach(func() {
				var err error
				vaultSettings := &v1.Settings_VaultSecrets{
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: "",
					},
				}
				clientAuth, err = ClientAuthFactory(vaultSettings)
				Expect(err).NotTo(HaveOccurred())
			})

			It("login should return an error", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).To(MatchError(ErrEmptyToken))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
			})

			It("startRenewal should return nil", func() {
				err := clientAuth.StartRenewal(ctx, client, nil)
				Expect(err).NotTo(HaveOccurred())
			})

		})

		When("token is not empty", func() {

			BeforeEach(func() {
				vaultSettings := &v1.Settings_VaultSecrets{
					AuthMethod: &v1.Settings_VaultSecrets_AccessToken{
						AccessToken: "placeholder",
					},
				}

				clientAuth, _ = ClientAuthFactory(vaultSettings)
			})

			It("should return a Secret", func() {
				secret, err := clientAuth.Login(ctx, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret).To(Equal(&vaultapi.Secret{
					Auth: &vaultapi.SecretAuth{
						ClientToken: "placeholder",
					},
				}))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			})

			It("startRenewal should return nil", func() {
				err := clientAuth.StartRenewal(ctx, client, nil)
				Expect(err).NotTo(HaveOccurred())
			})

		})

	})

	Context("NewRemoteTokenAuth", func() {
		// These tests validate the behavior of the RemoteTokenAuth implementation of the ClientAuth interface

		When("internal auth method always returns an error", func() {

			BeforeEach(func() {
				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("mocked error message")).AnyTimes()

				mockTokenRenewer = mock_vault.NewMockTokenRenewer(ctrl)
				mockTokenRenewer.EXPECT().StartRenewal(ctx, nil, nil).Return(nil).Times(0)

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, mockTokenRenewer, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))
			})

			It("should return the error", func() {
				secret, err := NewAuthenticatedClient(ctx, nil, clientAuth)
				Expect(err).To(MatchError("unable to log in to auth method: unable to authenticate to vault: mocked error message"))
				Expect(secret).To(BeNil())

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(3))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(0))
			})

		})

		XWhen("internal auth method returns an error, and then a success", func() {
			var (
				client *vaultapi.Client
				err    error
			)

			BeforeEach(func() {
				client = nil
				err = nil

				secret := &vaultapi.Secret{
					Auth: &vaultapi.SecretAuth{
						ClientToken: "a-client-token",
					},
				}

				ctrl = gomock.NewController(GinkgoT())
				internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).Times(1)
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).Times(1)

				mockTokenRenewer = mock_vault.NewMockTokenRenewer(ctrl)
				// this is the line that needs cleaning up
				mockTokenRenewer.EXPECT().StartRenewal(ctx, nil, secret).Return(nil).Times(1)

				clientAuth = NewRemoteTokenAuth(internalAuthMethod, mockTokenRenewer, &v1.Settings_VaultAwsAuth{}, retry.Attempts(5))
			})

			It("should return a client", func() {
				client, err = NewAuthenticatedClient(ctx, nil, clientAuth)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).ToNot((BeNil()))

				assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
				assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
				assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
				assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			})
		})

	})

	When("context is cancelled before login succeeds", func() {

		retryAttempts := uint(5)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			// The auth method will return an error twice, and then a success
			// but we plan on cancelling the context before the success
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, eris.New("error")).AnyTimes()
			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(retryAttempts))
		})

		It("should return a context error", func() {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()

			client, err := NewAuthenticatedClient(ctx, nil, clientAuth)
			Expect(err).To(MatchError("unable to log in to auth method: Login canceled: context canceled"))
			Expect(client).To(BeNil())

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			// Validate that the number of login failures is less than the number of retry attempts
			// This means we stopped the login attempts before they were exhausted
			assertions.ExpectStatSumMatches(MLoginFailures, BeNumerically("<", retryAttempts))
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())

		})

	})

})
