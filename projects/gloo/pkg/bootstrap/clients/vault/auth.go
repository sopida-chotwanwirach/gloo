package vault

import (
	"context"
	"math/rand"
	"time"

	"github.com/rotisserie/eris"
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/awsutils"

	"github.com/avast/retry-go"
	vault "github.com/hashicorp/vault/api"
	awsauth "github.com/hashicorp/vault/api/auth/aws"
	"github.com/solo-io/gloo/pkg/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
)

// In an ideal world, we would re-use the mocks provided by an external library.
// Since the vault.AuthMethod interface does not have corresponding mocks, we have to define our own.
//go:generate mockgen -destination mocks/mock_auth.go -package mocks github.com/hashicorp/vault/api AuthMethod

type ClientAuth interface {
	vault.AuthMethod
	// Start Renewal should be called after a successful login to start the renewal process
	// This method may have many different types of implementation, from just a noop to spinning up a separate go routine
	StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) error
}

var _ ClientAuth = &StaticTokenAuth{}
var _ ClientAuth = &RemoteTokenAuth{}

var (
	ErrEmptyToken          = errors.New("unable to authenticate to vault with empty token")
	ErrNoAuthInfo          = errors.New("no auth info was returned after login")
	ErrVaultAuthentication = func(err error) error {
		return errors.Wrap(err, "unable to authenticate to vault")
	}
	ErrPartialCredentials = func(err error) error {
		return eris.Wrap(err, "only partial credentials were provided for AWS IAM auth: ")
	}
	ErrAccessKeyId       = errors.New("access key id must be defined for AWS IAM auth")
	ErrSecretAccessKey   = errors.New("secret access key must be defined for AWS IAM auth")
	ErrInitializeWatcher = func(err error) error {
		return errors.Wrap(err, "unable to initialize new lifetime watcher for renewing auth token.")
	}
)

// ClientAuthFactory returns a vault ClientAuth based on the provided settings.
func ClientAuthFactory(vaultSettings *v1.Settings_VaultSecrets) (ClientAuth, error) {
	switch authMethod := vaultSettings.GetAuthMethod().(type) {
	case *v1.Settings_VaultSecrets_AccessToken:
		return NewStaticTokenAuth(authMethod.AccessToken), nil

	case *v1.Settings_VaultSecrets_Aws:
		awsAuth, err := newAwsAuthMethod(authMethod.Aws)
		if err != nil {
			return nil, err
		}

		return NewRemoteTokenAuth(awsAuth, authMethod.Aws), nil

	default:
		// AuthMethod is the preferred API to define the policy for authenticating to vault
		// If one is not defined, we fall back to the deprecated API
		return NewStaticTokenAuth(vaultSettings.GetToken()), nil
	}
}

// NewStaticTokenAuth is a constructor for StaticTokenAuth
func NewStaticTokenAuth(token string) ClientAuth {
	return &StaticTokenAuth{
		token: token,
	}
}

type StaticTokenAuth struct {
	token string
}

// GetToken returns the value of the token field
func (s *StaticTokenAuth) GetToken() string {
	return s.token
}

func (s *StaticTokenAuth) StartRenewal(_ context.Context, client *vault.Client, _ *vault.Secret) error {
	// static tokens do not need renewal
	return nil
}

// Login logs in to vault using a static token
func (s *StaticTokenAuth) Login(ctx context.Context, _ *vault.Client) (*vault.Secret, error) {
	if s.GetToken() == "" {
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrEmptyToken
	}

	contextutils.LoggerFrom(ctx).Debug("successfully authenticated to vault with static token")
	utils.Measure(ctx, MLastLoginSuccess, time.Now().Unix())
	utils.MeasureOne(ctx, MLoginSuccesses)
	return &vault.Secret{
		Auth: &vault.SecretAuth{
			ClientToken: s.token,
		},
	}, nil
}

// NewRemoteTokenAuth is a constructor for RemoteTokenAuth
func NewRemoteTokenAuth(authMethod vault.AuthMethod, aws *v1.Settings_VaultAwsAuth, retryOptions ...retry.Option) ClientAuth {

	// Standard retry options, which can be overridden by the loginRetryOptions parameter
	defaultRetryOptions := []retry.Option{
		retry.Delay(1 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.Attempts(1),
		retry.LastErrorOnly(true),
	}

	loginRetryOptions := append(defaultRetryOptions, retryOptions...)

	return &RemoteTokenAuth{
		authMethod:        authMethod,
		loginRetryOptions: loginRetryOptions,
		leaseIncrement:    int(aws.GetLeaseIncrement()),
	}
}

type RemoteTokenAuth struct {
	authMethod        vault.AuthMethod
	loginRetryOptions []retry.Option
	leaseIncrement    int
}

// Login logs into vault using the provided authMethod
func (r *RemoteTokenAuth) Login(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	var (
		loginResponse *vault.Secret
		loginErr      error
	)

	// Set the "retryIf" option here. We don't want this to be overridden, and the context isn't
	// available in the contructor to configure this
	retryOptions := append(
		r.loginRetryOptions,
		retry.RetryIf(func(err error) bool {
			// if the parent context is cancelled,
			// stop retrying.
			select {
			case <-ctx.Done():
				return false
			default:
				return true
			}
		}),
	)

	loginErr = retry.Do(func() error {
		loginResponse, loginErr = r.loginOnce(ctx, client)
		return loginErr
	}, retryOptions...)

	// As noted above, we need to check the context here, because our retry function can not return errors
	if ctx.Err() != nil {
		return nil, eris.Wrap(ctx.Err(), "Login canceled")
	}

	return loginResponse, loginErr
}

func (r *RemoteTokenAuth) loginOnce(ctx context.Context, client *vault.Client) (*vault.Secret, error) {
	loginResponse, loginErr := r.authMethod.Login(ctx, client)
	if loginErr != nil {
		contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to vault: %v", loginErr)
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrVaultAuthentication(loginErr)
	}

	if loginResponse == nil {
		contextutils.LoggerFrom(ctx).Error(ErrNoAuthInfo)
		utils.Measure(ctx, MLastLoginFailure, time.Now().Unix())
		utils.MeasureOne(ctx, MLoginFailures)
		return nil, ErrNoAuthInfo
	}

	contextutils.LoggerFrom(ctx).Debugf("successfully authenticated to vault %v", loginResponse)
	utils.Measure(ctx, MLastLoginSuccess, time.Now().Unix())
	utils.MeasureOne(ctx, MLoginSuccesses)
	return loginResponse, nil
}

func (r *RemoteTokenAuth) StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) error {
	go r.renewToken(ctx, client, secret)
	return nil
}

func newAwsAuthMethod(aws *v1.Settings_VaultAwsAuth) (*awsauth.AWSAuth, error) {
	// The AccessKeyID and SecretAccessKey are not required in the case of using temporary credentials from assumed roles with AWS STS or IRSA.
	// STS: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_use-resources.html
	// IRSA: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
	var possibleErrors []error
	if accessKeyId := aws.GetAccessKeyId(); accessKeyId != "" {
		awsutils.SetAccessKeyEnv(accessKeyId)
	} else {
		possibleErrors = append(possibleErrors, ErrAccessKeyId)
	}

	if secretAccessKey := aws.GetSecretAccessKey(); secretAccessKey != "" {
		awsutils.SetSecretAccessKeyEnv(secretAccessKey)
	} else {
		possibleErrors = append(possibleErrors, ErrSecretAccessKey)
	}

	// if we have only partial configuration set
	if len(possibleErrors) == 1 {
		return nil, ErrPartialCredentials(possibleErrors[0])
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
		awsutils.SetSessionTokenEnv(sessionToken)
	}

	return awsauth.NewAWSAuth(loginOptions...)
}

// Once you've set the token for your Vault client, you will need to periodically renew its lease.
// taken from https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *RemoteTokenAuth) renewToken(ctx context.Context, client *vault.Client, secret *vault.Secret) {
	contextutils.LoggerFrom(ctx).Debugf("Starting renewToken goroutine")
	for {
		secret, err := r.Login(ctx, client) //vi.loginWithRetry(ctx, client, awsAuth, nil)
		// The only errors we should ever hit are context errors because we are retrying on all other errors
		if err != nil {
			if ctx.Err() != nil {
				contextutils.LoggerFrom(ctx).Errorf("renew token context error: %v", ctx.Err())
				return // ! we are now no longer renewing the token
			} else {
				// This should never happen because we are retrying on all non-context errors
				contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to Vault: %v. This error is expected to be unreachable.", err)
			}
		}

		retry, tokenErr := r.manageTokenLifecycle(ctx, client, secret, r.leaseIncrement)

		// The only error this function can return is if the vaultLoginResp is nil, and we have checked against that in loginWithRetry, which
		// is currently the only called.
		if tokenErr != nil {
			contextutils.LoggerFrom(ctx).Errorf("unable to start managing token lifecycle: %v. This error is expected to be unreachable.", tokenErr)
		}

		if !retry {
			// contextutils.LoggerFrom(ctx).Infof("Stopping renewToken goroutine")
			return
		}

	}

}

// Starts token lifecycle management
// otherwise returns nil so we can attempt login again.
// based on https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *RemoteTokenAuth) manageTokenLifecycle(ctx context.Context, client *vault.Client, secret *vault.Secret, watcherIncrement int) (bool, error) {
	// Make sure the token is renewable
	if renewable, err := secret.TokenIsRenewable(); !renewable || err != nil {
		// If the token is not renewable and we immediately try to renew it, we will just keep trying and hitting the same error
		// So we need to throw in a sleep
		retryOnNonRenwableSleep := watcherIncrement
		defaultRetry := 60
		if retryOnNonRenwableSleep == 0 {
			retryOnNonRenwableSleep = 60
		}

		contextutils.LoggerFrom(ctx).Errorw("Token is not configured to be renewable.", "retry", retryOnNonRenwableSleep, "Error", err, "TokenIsRenewable", renewable)

		// The units don't make sense but this is the way the docs recommend doing it
		time.Sleep(time.Duration(defaultRetry) * time.Second)

		// If we are caught in this loop, we don't get to the code that checks the state of the context, so we need to check it here
		retryLogin := ctx.Done() == nil
		return retryLogin, nil
	}

	watcher, err := client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    secret,
		Increment: watcherIncrement,
		// Below this comment we are manually setting the parameters to current defaults to protect against future changes
		Rand:          rand.New(rand.NewSource(int64(time.Now().Nanosecond()))),
		RenewBuffer:   5, // equivalent to vault.DefaultLifetimeWatcherRenewBuffer,
		RenewBehavior: vault.RenewBehaviorIgnoreErrors,
	})

	// The only errors the constructor can return are if the input parameter is nil or if the secret is nil, and we
	// are always passing input and have validated the secret is not nil in the calling
	if err != nil {
		return false, ErrInitializeWatcher(err)
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		// `DoneCh` will return if renewal fails, or if the remaining lease
		// duration is under a built-in threshold and either renewing is not
		// extending it or renewing is disabled. In any case, the caller
		// needs to attempt to log in again.
		case err := <-watcher.DoneCh():
			utils.Measure(ctx, MLastRenewFailure, time.Now().Unix())
			utils.MeasureOne(ctx, MRenewFailures)
			if err != nil {
				contextutils.LoggerFrom(ctx).Debugf("Failed to renew token: %v. Re-attempting login.", err)
				return true, nil
			}
			// This occurs once the token has reached max TTL.
			contextutils.LoggerFrom(ctx).Debugf("Token can no longer be renewed. Re-attempting login.")
			return true, nil

		// Successfully completed renewal
		case renewal := <-watcher.RenewCh():
			utils.Measure(ctx, MLastRenewSuccess, time.Now().Unix())
			utils.MeasureOne(ctx, MRenewSuccesses)
			contextutils.LoggerFrom(ctx).Debugf("Successfully renewed: %v.", renewal)

		case <-ctx.Done():
			return false, nil
		}

	}
}
