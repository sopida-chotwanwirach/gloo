package vault

import (
    "context"
    vault "github.com/hashicorp/vault/api"
)

/**

Construct a vault client
login to client

potentially open a thread to repeatedly login

*/

func NewClient(ctx context.Context) (*vault.Client, error) {
    // 1. Create a new Client
    // We can do this by using a constructor that accepts SEttings object, and returns a client or error
    var client *vault.Client

    // 2. Create a new AuthMethod
    // We can do this by using a constructor that accepts SEttings object, and returns an AuthMethod or error
    var authMethod vault.AuthMethod

    // 3. Create a new LoginMethod
    var loginMethod LoginMethod

    // 5. Start a thread to manage renewal
    err := loginMethod.Login(ctx, client, authMethod)

    return client, err
}
