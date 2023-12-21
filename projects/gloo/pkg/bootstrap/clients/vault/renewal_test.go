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

		doneCh  chan error
		renewCh chan *vault.RenewOutput

		sleepTime = 100 * time.Millisecond

		renewableSecret = func() *vault.Secret {
			return &vault.Secret{
				Auth: &vault.SecretAuth{
					Renewable: true,
				},
				LeaseDuration: 100,
			}
		}

		nonRenewableSecret = func() *vault.Secret {
			return &vault.Secret{
				Auth: &vault.SecretAuth{
					Renewable: false,
				},
				LeaseDuration: 100,
			}
		}
	)

	var getTestWatcher = func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, func(), error) {
		return tw, func() {}, nil
	}

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		secret = renewableSecret()
		resetViews()

		doneCh = make(chan error, 1)
		renewCh = make(chan *vault.RenewOutput, 1)

		tw = &testWatcher{
			DoneChannel:  doneCh,
			RenewChannel: renewCh,
		}

	})

	When("Login always succeeds", func() {

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(secret, nil).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})
		It("Renewal should work", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- eris.Errorf("Renewal error")
				time.Sleep(sleepTime)
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

	When("Login fails sometimes", func() {
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)

			loginCount := 0
			// Fail every other login
			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, _ *vault.Client) (*vault.Secret, error) {
				loginCount += 1
				if loginCount%2 == 0 {
					return secret, nil
				}

				return nil, errMock

			})

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})

		It("should work with failures captured in metrics", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- eris.Errorf("Renewal error")
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(1 * time.Second) // A little extra sleep to let logins retry
				cancel()
			}()

			renewer.RenewToken(ctx, client, secret)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, Equal(1))
			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(1))
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(2))
		})
	})

	When("Login always fails", func() {
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)

			internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Return(nil, errMock).AnyTimes()

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})

		})

		It("Should renew once then get stuck on the login failure", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				doneCh <- eris.Errorf("Renewal error")
				time.Sleep(sleepTime)
				renewCh <- &vault.RenewOutput{}
				//time.Sleep(3 * time.Second) // A little extra sleep to let logins retry
				time.Sleep(sleepTime)
				cancel()
			}()

			renewer.RenewToken(ctx, client, secret)

			assertions.ExpectStatLastValueMatches(MLastLoginFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, BeZero())
			assertions.ExpectStatSumMatches(MLoginFailures, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginSuccesses, BeZero())
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			// We only get one success because we're blocked after the first failure
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})

	When("There is a non-renewable token then the token is updated", func() {

		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			internalAuthMethod := mocks.NewMockAuthMethod(ctrl)

			gomock.InOrder(
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).Times(1).Return(nonRenewableSecret(), nil),
				internalAuthMethod.EXPECT().Login(ctx, gomock.Any()).AnyTimes().Return(renewableSecret(), nil),
			)

			clientAuth = NewRemoteTokenAuth(internalAuthMethod, &NoOpRenewal{}, &v1.Settings_VaultAwsAuth{}, retry.Attempts(3))

			renewer = NewVaultTokenRenewer(&NewVaultTokenRenewerParams{
				Auth:           clientAuth,
				LeaseIncrement: 1,
				GetWatcher:     getTestWatcher,
			})
		})

		It("should work when the secret is updated to be renewable", func() {
			// Run through the basic channel output and look at the metrics
			go func() {
				time.Sleep(sleepTime)
				doneCh <- eris.Errorf("Renewal error") // Force renewal
				time.Sleep(2 * time.Second)            // Give it time to retry the login
				renewCh <- &vault.RenewOutput{}
				time.Sleep(sleepTime)
				cancel()
			}()

			renewer.RenewToken(ctx, client, nonRenewableSecret())

			// The login never fails, it just returns an non-renewable secret
			assertions.ExpectStatLastValueMatches(MLastLoginFailure, BeZero())
			assertions.ExpectStatLastValueMatches(MLastLoginSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MLoginFailures, BeZero())
			// Log in once for the first, passed in unrenewable secret,
			// then again for the unrenewable from the mocked response and then again for the success
			assertions.ExpectStatSumMatches(MLoginSuccesses, Equal(3))
			assertions.ExpectStatLastValueMatches(MLastRenewFailure, Not(BeZero()))
			assertions.ExpectStatLastValueMatches(MLastRenewSuccess, Not(BeZero()))
			assertions.ExpectStatSumMatches(MRenewFailures, Equal(1))
			assertions.ExpectStatSumMatches(MRenewSuccesses, Equal(1))
		})
	})
})
