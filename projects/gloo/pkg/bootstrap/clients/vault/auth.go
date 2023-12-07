package vault

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/avast/retry-go"
	vault "github.com/hashicorp/vault/api"
	awsauth "github.com/hashicorp/vault/api/auth/aws"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
)

var _ vault.AuthMethod = &staticTokenAuthMethod{}
var _ vault.AuthMethod = &awsAuthMethod{}

var (
	mLastLoginSuccess = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_login_success", "Timestamp of last successful authentication of vault with AWS IAM")
	mLastLoginFailure = utils.MakeLastValueCounter("gloo.solo.io/vault/aws/last_login_failure", "Timestamp of last failed authentication of vault with AWS IAM")
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
	if s.token == "" {
		return nil, eris.Errorf("unable to determine vault authentication method. check Settings configuration")
	}

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

func newAuthMethodForSettings(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets) (vault.AuthMethod, error) {
	// each case returns
	switch tlsCfg := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		return &staticTokenAuthMethod{token: tlsCfg.AccessToken}, nil

	case *v1.Settings_VaultSecrets_Aws:
		awsAuth, err := configureAwsAuth(ctx, tlsCfg.Aws)
		if err != nil {
			return nil, err
		}

		return &awsAuthMethod{
			awsAuth: awsAuth,
		}, nil

	default:
		// We don't have one of the defined auth methods, so try to fall back to the
		// deprecated token field before erroring
		return &staticTokenAuthMethod{token: vaultSettings.GetToken()}, nil
	}
}

func configureAwsAuth(ctx context.Context, aws *v1.Settings_VaultAwsAuth) (*awsauth.AWSAuth, error) {
	// The AccessKeyID and SecretAccessKey are not required in the case of using temporary credentials from assumed roles with AWS STS or IRSA.
	// STS: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html
	// IRSA: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	var possibleErrStrings []string
	if accessKeyId := aws.GetAccessKeyId(); accessKeyId != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyId)
	} else {
		possibleErrStrings = append(possibleErrStrings, "access key id must be defined for AWS IAM auth")
	}

	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)
	} else {
		possibleErrStrings = append(possibleErrStrings, "secret access key must be defined for AWS IAM auth")
	}

	// if we have only partial configuration set
	if len(possibleErrStrings) == 1 {
		return nil, errors.New("only partial credentials were provided for AWS IAM auth: " + possibleErrStrings[0])
	}

	// At this point, we either have full auth configuration set, or are in an ec2 environment, where vault will infer the credentials.
	loginOptions := []awsauth.LoginOption{awsauth.WithIAMAuth()}

	if role := aws.GetVaultRole(); role != "" {
		loginOptions = append(loginOptions, awsauth.WithRole(role))
	}

	if region := aws.GetRegion(); region != "" {
		loginOptions = append(loginOptions, awsauth.WithRegion(region))
	}

	if iamServerIdHeader := aws.GetIamServerIdHeader(); iamServerIdHeader != "" {
		loginOptions = append(loginOptions, awsauth.WithIAMServerIDHeader(iamServerIdHeader))
	}

	if mountPath := aws.GetMountPath(); mountPath != "" {
		loginOptions = append(loginOptions, awsauth.WithMountPath(mountPath))
	}

	if sessionToken := aws.GetSessionToken(); sessionToken != "" {
		os.Setenv("AWS_SESSION_TOKEN", sessionToken)
	}

	return awsauth.NewAWSAuth(loginOptions...)
}
