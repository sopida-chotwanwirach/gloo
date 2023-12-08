package vault

import (
    "context"

    vault "github.com/hashicorp/vault/api"
    v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
    "google.golang.org/protobuf/types/known/wrapperspb"
)

/**

Construct a vault client
login to client

potentially open a thread to repeatedly login

*/

func newClientForSettings(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets) (*vault.Client, error) {
    cfg, err := parseVaultSettings(vaultSettings)
    if err != nil {
        return nil, err
    }
    return vault.NewClient(cfg)
}

func parseVaultSettings(vaultSettings *v1.Settings_VaultSecrets) (*vault.Config, error) {
    cfg := vault.DefaultConfig()

    if addr := vaultSettings.GetAddress(); addr != "" {
        cfg.Address = addr
    }
    if tlsConfig := parseTlsSettings(vaultSettings); tlsConfig != nil {
        if err := cfg.ConfigureTLS(tlsConfig); err != nil {
            return nil, err
        }
    }

    return cfg, nil
}

func parseTlsSettings(vaultSettings *v1.Settings_VaultSecrets) *vault.TLSConfig {
    var tlsConfig *vault.TLSConfig

    // helper functions to avoid repeated nilchecking
    addStringSetting := func(s string, addSettingFunc func(string)) {
        if s == "" {
            return
        }
        if tlsConfig == nil {
            tlsConfig = &vault.TLSConfig{}
        }
        addSettingFunc(s)
    }
    addBoolSetting := func(b *wrapperspb.BoolValue, addSettingFunc func(bool)) {
        if b == nil {
            return
        }
        if tlsConfig == nil {
            tlsConfig = &vault.TLSConfig{}
        }
        addSettingFunc(b.GetValue())
    }

    setCaCert := func(s string) { tlsConfig.CACert = s }
    setCaPath := func(s string) { tlsConfig.CAPath = s }
    setClientCert := func(s string) { tlsConfig.ClientCert = s }
    setClientKey := func(s string) { tlsConfig.ClientKey = s }
    setTlsServerName := func(s string) { tlsConfig.TLSServerName = s }
    setInsecure := func(b bool) { tlsConfig.Insecure = b }

    // Add our settings to the vault TLS config, preferring settings set in the
    // new TlsConfig field if it is used to those in the deprecated fields
    if tlsSettings := vaultSettings.GetTlsConfig(); tlsSettings == nil {
        addStringSetting(vaultSettings.GetCaCert(), setCaCert)
        addStringSetting(vaultSettings.GetCaPath(), setCaPath)
        addStringSetting(vaultSettings.GetClientCert(), setClientCert)
        addStringSetting(vaultSettings.GetClientKey(), setClientKey)
        addStringSetting(vaultSettings.GetTlsServerName(), setTlsServerName)
        addBoolSetting(vaultSettings.GetInsecure(), setInsecure)
    } else {
        addStringSetting(vaultSettings.GetTlsConfig().GetCaCert(), setCaCert)
        addStringSetting(vaultSettings.GetTlsConfig().GetCaPath(), setCaPath)
        addStringSetting(vaultSettings.GetTlsConfig().GetClientCert(), setClientCert)
        addStringSetting(vaultSettings.GetTlsConfig().GetClientKey(), setClientKey)
        addStringSetting(vaultSettings.GetTlsConfig().GetTlsServerName(), setTlsServerName)
        addBoolSetting(vaultSettings.GetTlsConfig().GetInsecure(), setInsecure)
    }

    return tlsConfig

}

func NewClient(ctx context.Context, vaultSettings *v1.Settings_VaultSecrets) (*vault.Client, error) {
    // 1. Create a new Client
    // We can do this by using a constructor that accepts SEttings object, and returns a client or error
    var (
        client *vault.Client
        err    error
    )
    client, err = newClientForSettings(ctx, vaultSettings)
    if err != nil {
        return nil, err
    }

    // 2. Create a new AuthMethod
    // We can do this by using a constructor that accepts SEttings object, and returns an AuthMethod or error
    var authMethod vault.AuthMethod
    authMethod, err = NewAuthMethodForSettings(ctx, vaultSettings)
    if err != nil {
        return nil, err
    }

    // 3. Create a new LoginMethod
    var loginMethod LoginMethod

    // 5. Login and potentially open up goroutines to manage renewal
    err = loginMethod.Login(ctx, client, authMethod)

    return client, err
}
