package vault

import (
    "context"
    "errors"
    "github.com/avast/retry-go"
    vault "github.com/hashicorp/vault/api"
    awsauth "github.com/hashicorp/vault/api/auth/aws"
    "github.com/solo-io/gloo/pkg/utils"
    "github.com/solo-io/go-utils/contextutils"
    "time"
)

var _ vault.AuthMethod = &staticTokenAuthMethod{}
var _ vault.AuthMethod = &awsAuthMethod{}

var (
    mLastRenewSuccess = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_renew_success", "Timestamp of last successful renewal of vault secret lease")
    mLastRenewFailure = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_renew_failure", "Timestamp of last failed renewal of vault secret lease")
    mLastLoginSuccess = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_login_success", "Timestamp of last successful authentication of vault with AWS IAM")
    mLastLoginFailure = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_login_failure", "Timestamp of last failed authentication of vault with AWS IAM")
    mRenewSuccesses   = utils.MakeSumCounter("gloo.solo.io/vault/aws/renew_successes", "Number of successful renewals of vault secret lease")
    mRenewFailures    = utils.MakeSumCounter("gloo.solo.io/vault/aws/renew_failures", "Number of failed renewals of vault secret lease")
    mLoginSuccesses   = utils.MakeSumCounter("gloo.solo.io/vault/aws/login_successes", "Number of successful authentications of vault with AWS IAM")
    mLoginFailures    = utils.MakeSumCounter("gloo.solo.io/vault/aws/login_failures", "Number of failed authentications of vault with AWS IAM")
)

var (
    ErrVaultAuthentication = errors.New("unable to authenticate to Vault")
    ErrNoAuthInfo          = errors.New("no auth info was returned after login")
)

type staticTokenAuthMethod struct {
    token string
}

func (s *staticTokenAuthMethod) Login(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
    contextutils.LoggerFrom(ctx).Debugf("Successfully authenticated to Vault with static token")
    utils.Measure(ctx, mLastLoginSuccess, time.Now().Unix())
    utils.MeasureOne(ctx, mLoginSuccesses)
    return &vault.Secret{
        Auth: &vault.SecretAuth{
            ClientToken: s.token,
        },
    }, nil
}

type awsAuthMethod struct {
    awsAuth *awsauth.AWSAuth
}

func (a *awsAuthMethod) Login(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
    var (
        loginResponse *vault.Secret
        loginErr      error
    )

    loginOnce := func() error {
        loginResponse, loginErr = a.awsAuth.Login(ctx, client)
        if loginErr != nil {
            contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to Vault: %v", loginErr)
            utils.Measure(ctx, mLastLoginFailure, time.Now().Unix())
            utils.MeasureOne(ctx, mLoginFailures)
            return ErrVaultAuthentication
        }

        if loginResponse == nil {
            contextutils.LoggerFrom(ctx).Errorf("no auth info was returned after login")
            utils.Measure(ctx, mLastLoginFailure, time.Now().Unix())
            utils.MeasureOne(ctx, mLoginFailures)
            return ErrNoAuthInfo
        }

        utils.Measure(ctx, mLastLoginSuccess, time.Now().Unix())
        utils.MeasureOne(ctx, mLoginSuccesses)
        contextutils.LoggerFrom(ctx).Debugf("Successfully authenticated to Vault %v", loginResponse)
        return nil
    }

    // We build a retry loop here because connection to Vault's AWS auth backend requires a network call,
    // and we want to be resilient to transient network failures.
    retryOptions := []retry.Option{
        retry.Delay(1 * time.Second),
        retry.DelayType(retry.BackOffDelay),
    }

    loginErr = retry.Do(loginOnce, retryOptions...)
    return loginResponse, loginErr
}
