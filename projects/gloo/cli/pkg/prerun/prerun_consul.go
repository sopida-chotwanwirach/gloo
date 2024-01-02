package prerun

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
)

func EnableConsulClients(opts *options.Options) error {
	consul := opts.Top.Consul
	if consul.UseConsul {
		// Creating Consul's query options for the consul clients
		queryOptions := &api.QueryOptions{
			AllowStale:        consul.AllowStaleReads,
			RequireConsistent: !consul.AllowStaleReads,
		}

		client, err := consul.Client()
		if err != nil {
			return eris.Wrapf(err, "creating Consul client")
		}
		helpers.UseConsulClients(client, consul.RootKey, queryOptions)
	}
	return nil
}

func EnableVaultClients(ctx context.Context, vault options.Vault) error {
	if vault.UseVault {
		client, err := vault.Client()
		if err != nil {
			return eris.Wrapf(err, "creating Vault client")
		}
		helpers.UseVaultClients(ctx, client, vault.PathPrefix, vault.RootKey)
	}
	return nil
}
