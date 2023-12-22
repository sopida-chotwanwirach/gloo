package vault

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
)

type TokenWatcher interface {
	DoneCh() <-chan error
	RenewCh() <-chan *vault.RenewOutput
}

type TokenRenewer interface {
	StartRenewal(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret) error
}

var _ TokenRenewer = &VaultTokenRenewer{}

// getWatcherFunc is a function that returns a TokenWatcher and a function to stop the watcher
// this lets us hide away some go routines while testing
type getWatcherFunc func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, func(), error)
type VaultTokenRenewer struct {
	auth                     vault.AuthMethod
	leaseIncrement           int
	getWatcher               getWatcherFunc
	retryOnNonRenewableSleep int
}

type NewVaultTokenRenewerParams struct {
	// Auth provides the login method for the vault client to be used when the lease is up
	Auth vault.AuthMethod
	// LeaseIncrement is the amount of time in seconds for which the lease should be renewed
	LeaseIncrement int
	// A function to provide the watcher and provide a point to inject a test function for testing
	GetWatcher               getWatcherFunc
	RetryOnNonRenewableSleep int
}

// NewVaultTokenRenewer returns a new VaultTokenRenewer and will set the default GetWatcher Function
func NewVaultTokenRenewer(params *NewVaultTokenRenewerParams) *VaultTokenRenewer {
	if params.GetWatcher == nil {
		params.GetWatcher = vaultGetWatcher
	}

	// This is the amount of time to sleep before retrying if the token is not renewable
	if params.RetryOnNonRenewableSleep == 0 {
		params.RetryOnNonRenewableSleep = 60
	}

	return &VaultTokenRenewer{
		auth:           params.Auth,
		leaseIncrement: params.LeaseIncrement,
		getWatcher:     params.GetWatcher,
	}
}

// StartRenewal wraps the renewal process in a go routine
func (t *VaultTokenRenewer) StartRenewal(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret) error {
	go t.RenewToken(ctx, client, clientAuth, secret)
	return nil
}

// Once you've set the token for your Vault client, you will need to periodically renew its lease.
// taken from https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *VaultTokenRenewer) RenewToken(ctx context.Context, client *vault.Client, clientAuth ClientAuth, secret *vault.Secret) {
	contextutils.LoggerFrom(ctx).Debugf("Starting renewToken goroutine")

	// On the first time through we have just logged in, so we don't need to do it again. Every other time we need to log in again.
	haveValidSecret := true
	for {
		var err error
		if !haveValidSecret {
			fmt.Printf("Logging in again with %+v \n", clientAuth)
			secret, err = clientAuth.Login(ctx, client) //vi.loginWithRetry(ctx, client, awsAuth, nil)
			if err != nil {
				fmt.Printf("Error %v\n", err)
			} else {
				fmt.Printf("Sucessful login")
			}
		} else {
			haveValidSecret = false
		}

		if err != nil {
			if ctx.Err() != nil {
				contextutils.LoggerFrom(ctx).Errorf("renew token context error: %v", ctx.Err())
			} else {
				contextutils.LoggerFrom(ctx).Errorf("unable to authenticate to Vault: %v.", err)
			}
			return // we are now no longer renewing the token
		}

		retry, tokenErr := r.manageTokenLifecycle(ctx, client, secret, r.leaseIncrement)

		// The only error this function can return is if the vaultLoginResp is nil, and we have checked against that in loginWithRetry, which
		// is currently the only called.
		if tokenErr != nil {
			contextutils.LoggerFrom(ctx).Errorf("unable to start managing token lifecycle: %v. This error is expected to be unreachable.", tokenErr)
		}

		fmt.Print("returned from manageTokenLifecycle, logging in again")

		if !retry {
			return
		}

	}

}

var vaultGetWatcher = func(client *vault.Client, secret *vault.Secret, watcherIncrement int) (TokenWatcher, func(), error) {
	watcher, err := client.NewLifetimeWatcher(&vault.LifetimeWatcherInput{
		Secret:    secret,
		Increment: watcherIncrement,
		// Below this comment we are manually setting the parameters to current defaults to protect against future changes
		Rand:          rand.New(rand.NewSource(int64(time.Now().Nanosecond()))),
		RenewBuffer:   5, // equivalent to vault.DefaultLifetimeWatcherRenewBuffer,
		RenewBehavior: vault.RenewBehaviorIgnoreErrors,
	})

	if err != nil {
		return nil, nil, ErrInitializeWatcher(err)
	}

	go watcher.Start()

	return watcher, watcher.Stop, nil
}

// Starts token lifecycle management
// otherwise returns nil so we can attempt login again.
// based on https://github.com/hashicorp/vault-examples/blob/main/examples/token-renewal/go/example.go
func (r *VaultTokenRenewer) manageTokenLifecycle(ctx context.Context, client *vault.Client, secret *vault.Secret, watcherIncrement int) (bool, error) {

	// Make sure the token is renewable
	if renewable, err := secret.TokenIsRenewable(); !renewable || err != nil {
		// If the token is not renewable and we immediately try to renew it, we will just keep trying and hitting the same error
		// So we need to throw in a sleep

		contextutils.LoggerFrom(ctx).Errorw("Token is not configured to be renewable.", "retry", r.retryOnNonRenewableSleep, "Error", err, "TokenIsRenewable", renewable)

		// The units don't make sense but this is the way the docs recommend doing it
		time.Sleep(time.Duration(r.retryOnNonRenewableSleep) * time.Second)

		// If we are caught in this loop, we don't get to the code that checks the state of the context, so we need to check it here
		retryLogin := ctx.Err() == nil
		return retryLogin, nil
	}

	fmt.Println("watcherIncrement is ", watcherIncrement)
	watcher, stop, err := r.getWatcher(client, secret, watcherIncrement)

	// The only errors the constructor can return are if the input parameter is nil or if the secret is nil, and we
	// are always passing input and have validated the secret is not nil in the calling
	if err != nil {
		return false, ErrInitializeWatcher(err)
	}

	defer stop()

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
				fmt.Printf("Failed to renew token: %v. Re-attempting login.\n", err)
				contextutils.LoggerFrom(ctx).Debugf("Failed to renew token: %v. Re-attempting login.", err)
				return true, nil
			}
			// This occurs once the token has reached max TTL.
			fmt.Printf("Token can no longer be renewed. Re-attempting login.\n")
			contextutils.LoggerFrom(ctx).Debugf("Token can no longer be renewed. Re-attempting login.")
			return true, nil

		// Successfully completed renewal
		case renewal := <-watcher.RenewCh():
			utils.Measure(ctx, MLastRenewSuccess, time.Now().Unix())
			utils.MeasureOne(ctx, MRenewSuccesses)
			fmt.Printf("Successfully renewed: %v.\n", renewal)
			contextutils.LoggerFrom(ctx).Debugf("Successfully renewed: %v.", renewal)

		case <-ctx.Done():
			return false, nil
		}

	}
}
