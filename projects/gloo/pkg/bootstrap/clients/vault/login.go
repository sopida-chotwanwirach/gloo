package vault

import (
    "context"
    vault "github.com/hashicorp/vault/api"
    "time"
)

var _ LoginMethod = &neverRenewMethod{}
var _ LoginMethod = &timedRenewMethod{}
var _ LoginMethod = &watchedRenewMethod{}

type LoginMethod interface {
    Login(ctx context.Context, client *vault.Client, authMethod vault.AuthMethod) error
}

type neverRenewMethod struct {
}

func (n *neverRenewMethod) Login(ctx context.Context, client *vault.Client, authMethod vault.AuthMethod) error {
    _, err := client.Auth().Login(ctx, authMethod)
    return err
}

type timedRenewMethod struct {
    interval time.Duration
}

func (t *timedRenewMethod) Login(ctx context.Context, client *vault.Client, authMethod vault.AuthMethod) error {
    // todo - use a sleep -based watcher
    return nil
}

type watchedRenewMethod struct {
    // todo
}

func (w *watchedRenewMethod) Login(ctx context.Context, client *vault.Client, authMethod vault.AuthMethod) error {
    // todo = use the lifetime watcher
    return nil
}
