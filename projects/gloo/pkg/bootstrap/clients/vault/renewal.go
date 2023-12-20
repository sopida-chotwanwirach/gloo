package vault

import (
	"context"
	"math/rand"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

type TokenRenewer interface {
	// Start Renewal should be called after a successful login to start the renewal process
	// This method may have many different types of implementation, from just a noop to spinning up a separate go routine
	StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) error
}

type VaultTokenRenewer struct {
	auth           vault.AuthMethod
	leaseIncrement int
}

type NewVaultTokenRenewerParams struct {
	Auth           vault.AuthMethod
	LeaseIncrement int
}

func NewVaultTokenRenewer(params *NewVaultTokenRenewerParams) *VaultTokenRenewer {
	return &VaultTokenRenewer{
		auth:           params.Auth,
		leaseIncrement: params.LeaseIncrement,
	}
}

func (t *VaultTokenRenewer) StartRenewal(ctx context.Context, client *vault.Client, secret *vault.Secret) error {
	go t.RenewToken(ctx, client, secret)
	return nil
}

// Once you've set the token for your Vault client, you will need to periodically renew its lease.
// taken from https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *VaultTokenRenewer) RenewToken(ctx context.Context, client *vault.Client, secret *vault.Secret) {
	contextutils.LoggerFrom(ctx).Debugf("Starting renewToken goroutine")

	// On the first time through we have just logged in, so we don't need to do it again. Every other time we need to log in again.
	haveValidSecret := true
	for {
		var err error
		if !haveValidSecret {
			secret, err = r.auth.Login(ctx, client) //vi.loginWithRetry(ctx, client, awsAuth, nil)
		} else {
			haveValidSecret = false
		}
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
func (r *VaultTokenRenewer) manageTokenLifecycle(ctx context.Context, client *vault.Client, secret *vault.Secret, watcherIncrement int) (bool, error) {
	// Make sure the token is renewable
	// if renewable, err := secret.TokenIsRenewable(); !renewable || err != nil {
	// 	// If the token is not renewable and we immediately try to renew it, we will just keep trying and hitting the same error
	// 	// So we need to throw in a sleep
	// 	retryOnNonRenwableSleep := watcherIncrement
	// 	defaultRetry := 60
	// 	if retryOnNonRenwableSleep == 0 {
	// 		retryOnNonRenwableSleep = 60
	// 	}

	// 	contextutils.LoggerFrom(ctx).Errorw("Token is not configured to be renewable.", "retry", retryOnNonRenwableSleep, "Error", err, "TokenIsRenewable", renewable)

	// 	// The units don't make sense but this is the way the docs recommend doing it
	// 	time.Sleep(time.Duration(defaultRetry) * time.Second)

	// 	// If we are caught in this loop, we don't get to the code that checks the state of the context, so we need to check it here
	// 	retryLogin := ctx.Done() == nil
	// 	return retryLogin, nil
	// }

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
