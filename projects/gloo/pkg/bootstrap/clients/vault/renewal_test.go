package vault_test

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	"github.com/golang/mock/gomock"
	vault "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rotisserie/eris"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	. "github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap/clients/vault/mocks"
	"github.com/solo-io/gloo/test/gomega/assertions"
)

type testWatcher struct {
	DoneChannel  chan error
	RenewChannel chan *vault.RenewOutput
}

func (t *testWatcher) DoneCh() <-chan error {
	return t.DoneChannel
}

func (t *testWatcher) RenewCh() <-chan *vault.RenewOutput {
	return t.RenewChannel
}

var _ = Describe("Vault Token Renewal", func() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		client  *vault.Client
		renewer *VaultTokenRenewer
		secret  *vault.Secret

		clientAuth ClientAuth
		ctrl       *gomock.Controller
		errMock    = eris.New("mocked error message")
		tw         TokenWatcher
	)

	var getMockWatcher = func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, error) {
		return tw, nil
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		secret = &vault.Secret{
			Auth: &vault.SecretAuth{
				Renewable: true,
			},
			LeaseDuration: 100,
		}

		resetViews()
	})

	AfterEach(func() {
		cancel()
	})

	When("Login fails sometimes", func() {
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, errMock).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getMockWatcher,
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
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, BeZero())
			assertions.ExpectStatSumMatches(MRenewFailures, BeZero())
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})

	When("Login always succeeds", func() {
		var (
			doneCh  chan error
			renewCh chan *vault.RenewOutput
		)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			doneCh = make(chan error, 1)
			renewCh = make(chan *vault.RenewOutput, 1)

			tw = &testWatcher{
				DoneChannel:  doneCh,
				RenewChannel: renewCh,
			}

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getMockWatcher,
			})

			resetViews()
		})
		It("Renewal should work", func() {
			// We are going to send a renewal, then a failure, then a renewal, then a cancel
			// This tests all possible channel outputs. We'll look to see the correct metrics were collected
			go func() {
				time.Sleep(1 * time.Second)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(1 * time.Second)
				doneCh <- eris.Errorf("Renewal error")
				time.Sleep(1 * time.Second)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(1 * time.Second)
				cancel()
			}()

			renewer.RenewToken(ctx, client, secret)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(2))
		})
	})
})
