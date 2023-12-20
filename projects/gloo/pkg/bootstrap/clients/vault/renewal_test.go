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
	"github.com/solo-io/gloo/mocks"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

var _ = FDescribe("Vault Token Renewal", func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		client  *vaultapi.Client
		renewer *VaultTokenRenewer
		secret  *vaultapi.Secret

		clientAuth ClientAuth
		ctrl       *gomock.Controller
		errMock    = eris.New("mocked error message")
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		secret = &vaultapi.Secret{
			Renewable:     true,
			LeaseDuration: 100,
		}

		resetViews()
	})

	AfterEach(func() {
		cancel()
	})

	When("Login fails", func() {
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, errMock).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
			})

			resetViews()
		})

		It("should have a bunch of failures", func() {
			go func() {
				time.Sleep(5 * time.Second)
				cancel()
			}()

			renewer.RenewToken(ctx, client, secret)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			assertions.ExpectStatSumMatches(MLoginFailures, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
			// assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			// assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
			// assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
			// assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})

	FWhen("Everythign is running smooth", func() {
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
			})

			resetViews()
		})
		It("Renewal should work", func() {
			go func() {
				time.Sleep(15 * time.Second)
				cancel()
			}()

			renewer.RenewToken(ctx, client, secret)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(MLoginSuccesses, Not(BeZero()))
			// assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			// assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
			// assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
			// assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})
})
