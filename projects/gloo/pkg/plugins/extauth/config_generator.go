package extauth

import (
	"context"
	"strings"
	"time"

	"github.com/solo-io/gloo/pkg/utils/regexutils"

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	extauthv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/extauth/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/extauth"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

const JWTFilterName = "envoy.filters.http.jwt_authn"

var (
	DefaultTimeout = prototime.DurationToProto(200 * time.Millisecond)
	NoServerRefErr = eris.New("no extauth server reference configured")
	ServerNotFound = func(usRef *core.ResourceRef) error {
		return eris.Errorf("extauth server upstream not found %s", usRef.String())
	}
	InvalidStatusOnErrorErr = func(code uint32) error {
		return eris.Errorf("invalid statusOnError code: %d", code)
	}
)

// ExtAuthzConfigGenerator is responsible for generating the ext_authz filter configuration for a various resources
// It is responsible for generating the configuration for the listener, virtual host, route, and weighted destination.
// It assumes that the calling code (likely a plugin) has already determined that the ext_authz filter should be configured
type ExtAuthzConfigGenerator interface {
	IsMulti() bool
	GenerateListenerExtAuthzConfig(listener *v1.HttpListener, upstreams v1.UpstreamList) ([]*envoyauth.ExtAuthz, error)
	GenerateVirtualHostExtAuthzConfig(virtualHost *v1.VirtualHost, params plugins.VirtualHostParams) (*envoyauth.ExtAuthzPerRoute, error)
	GenerateRouteExtAuthzConfig(route *v1.Route) (*envoyauth.ExtAuthzPerRoute, error)
	GenerateWeightedDestinationExtAuthzConfig(weightedDestination *v1.WeightedDestination) (*envoyauth.ExtAuthzPerRoute, error)
}

func getOpenSourceConfigGenerator(defaultSettings *extauthv1.Settings, namedSettings map[string]*extauthv1.Settings) ExtAuthzConfigGenerator {
	if namedSettings == nil {
		return NewDefaultConfigGenerator(defaultSettings)
	}

	return NewMultiConfigGenerator()
}

type DefaultConfigGenerator struct {
	defaultSettings *extauthv1.Settings
}

func NewDefaultConfigGenerator(defaultSettings *extauthv1.Settings) *DefaultConfigGenerator {
	return &DefaultConfigGenerator{
		defaultSettings: defaultSettings,
	}
}

func (d *DefaultConfigGenerator) IsMulti() bool {
	// This generator is only responsible for creating a single ext_authz filter
	return false
}

func (d *DefaultConfigGenerator) GenerateListenerExtAuthzConfig(listener *v1.HttpListener, upstreams v1.UpstreamList) ([]*envoyauth.ExtAuthz, error) {
	// If extauth isn't defined on the listener, fallback to the default extauth settings
	settings := listener.GetOptions().GetExtauth()
	if settings == nil {
		settings = d.defaultSettings
	}

	// If extauth isn't defined on the listener or default settings, no extauth is configured
	if settings == nil {
		return nil, nil
	}

	extAuthCfg, err := GenerateEnvoyConfigForFilter(settings, upstreams)
	if err != nil {
		return nil, err
	}

	return []*envoyauth.ExtAuthz{extAuthCfg}, nil
}

func (d *DefaultConfigGenerator) GenerateVirtualHostExtAuthzConfig(virtualHost *v1.VirtualHost, params plugins.VirtualHostParams) (*envoyauth.ExtAuthzPerRoute, error) {
	extension := virtualHost.GetOptions().GetExtauth()
	if extension == nil {
		// This code is executed only if a ext_authz filter is configured on the FilterChain
		// If there is no extension defined for the virtual host, we disable authz for this virtual host
		return GetDisabledAuth(), nil
	}

	// If extauth is explicitly disabled on this virtual host, disable it
	if extension.GetDisable() {
		return GetDisabledAuth(), nil
	}

	customAuthConfig := extension.GetCustomAuth()

	// No extauth config on this virtual host, disable it
	if customAuthConfig == nil {
		return GetDisabledAuth(), nil
	}

	config := &envoyauth.ExtAuthzPerRoute{
		Override: &envoyauth.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoyauth.CheckSettings{
				ContextExtensions: customAuthConfig.GetContextExtensions(),
			},
		},
	}
	return config, nil
}

func (d *DefaultConfigGenerator) GenerateRouteExtAuthzConfig(route *v1.Route) (*envoyauth.ExtAuthzPerRoute, error) {
	extension := route.GetOptions().GetExtauth()
	if extension == nil {
		// This logic is different from that of the VirtualHost
		// If no extension is defined, we don't need to configure PerRoute configuration for this Route,
		// and the route will just inherit the VirtualHost configuration
		return nil, nil
	}

	// If extauth is explicitly disabled on this route, disable it
	if extension.GetDisable() {
		return GetDisabledAuth(), nil
	}

	customAuthConfig := extension.GetCustomAuth()

	// No custom config, do nothing
	if customAuthConfig == nil {
		return nil, nil
	}

	config := &envoyauth.ExtAuthzPerRoute{
		Override: &envoyauth.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoyauth.CheckSettings{
				ContextExtensions: customAuthConfig.GetContextExtensions(),
			},
		},
	}
	return config, nil
}

func (d *DefaultConfigGenerator) GenerateWeightedDestinationExtAuthzConfig(weightedDestination *v1.WeightedDestination) (*envoyauth.ExtAuthzPerRoute, error) {
	extension := weightedDestination.GetOptions().GetExtauth()
	if extension == nil {
		// This logic is different from that of the VirtualHost
		// If no extension is defined, we don't need to configure PerRoute configuration for this WeightedDestination,
		// and the destination will just inherit the VirtualHost configuration
		return nil, nil
	}

	// If extauth is explicitly disabled on this weighted destination, disable it
	if extension.GetDisable() {
		return GetDisabledAuth(), nil
	}

	customAuthConfig := extension.GetCustomAuth()

	// No custom config, do nothing
	if customAuthConfig == nil {
		return nil, nil
	}

	config := &envoyauth.ExtAuthzPerRoute{
		Override: &envoyauth.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &envoyauth.CheckSettings{
				ContextExtensions: customAuthConfig.GetContextExtensions(),
			},
		},
	}
	return config, nil
}

func GetDisabledAuth() *envoyauth.ExtAuthzPerRoute {
	return &envoyauth.ExtAuthzPerRoute{
		Override: &envoyauth.ExtAuthzPerRoute_Disabled{
			Disabled: true,
		},
	}
}

type MultiConfigGenerator struct {
	*DefaultConfigGenerator
}

func NewMultiConfigGenerator() *MultiConfigGenerator {
	return &MultiConfigGenerator{}
}

func (m *MultiConfigGenerator) IsMulti() bool {
	return true
}

func (m *MultiConfigGenerator) GenerateListenerExtAuthzConfig(listener *v1.HttpListener, upstreams v1.UpstreamList) ([]*envoyauth.ExtAuthz, error) {
	return nil, extauth.ErrEnterpriseOnly
}

func BuildStagedHttpFilters(configurationGenerator func() ([]*envoyauth.ExtAuthz, error), stage plugins.FilterStage) ([]plugins.StagedHttpFilter, error) {
	var filters []plugins.StagedHttpFilter

	configurations, err := configurationGenerator()
	if err != nil {
		return nil, err
	}

	for _, extAuthCfg := range configurations {
		stagedFilter, err := plugins.NewStagedFilter(wellknown.HTTPExternalAuthorization, extAuthCfg, stage)
		if err != nil {
			return nil, err
		}

		filters = append(filters, stagedFilter)
	}

	return filters, nil
}

func GenerateEnvoyConfigForFilter(settings *extauthv1.Settings, upstreams v1.UpstreamList) (*envoyauth.ExtAuthz, error) {
	extauthUpstreamRef := settings.GetExtauthzServerRef()
	if extauthUpstreamRef == nil {
		return nil, NoServerRefErr
	}

	// Make sure the server exists
	_, err := upstreams.Find(extauthUpstreamRef.GetNamespace(), extauthUpstreamRef.GetName())
	if err != nil {
		return nil, ServerNotFound(extauthUpstreamRef)
	}

	cfg := &envoyauth.ExtAuthz{
		MetadataContextNamespaces: []string{JWTFilterName},
	}

	httpService := settings.GetHttpService()
	if httpService == nil {
		svc := &envoycore.GrpcService{
			TargetSpecifier: &envoycore.GrpcService_EnvoyGrpc_{
				EnvoyGrpc: &envoycore.GrpcService_EnvoyGrpc{
					ClusterName: translator.UpstreamToClusterName(extauthUpstreamRef),
				},
			}}

		timeout := settings.GetRequestTimeout()
		if timeout == nil {
			timeout = DefaultTimeout
		}
		svc.Timeout = timeout

		grpcService := settings.GetGrpcService()
		if grpcService != nil && grpcService.GetAuthority() != "" {
			svc.GetEnvoyGrpc().Authority = grpcService.GetAuthority()
		}
		cfg.Services = &envoyauth.ExtAuthz_GrpcService{
			GrpcService: svc,
		}
	} else {
		httpURI := &envoycore.HttpUri{
			// This uri is not used by the filter but is required because of envoy validation.
			Uri:     HttpServerUri,
			Timeout: settings.GetRequestTimeout(),
			HttpUpstreamType: &envoycore.HttpUri_Cluster{
				Cluster: translator.UpstreamToClusterName(extauthUpstreamRef),
			},
		}
		if httpURI.GetTimeout() == nil {
			// Set to the default. This is required by envoy validation.
			httpURI.Timeout = DefaultTimeout
		}

		cfg.Services = &envoyauth.ExtAuthz_HttpService{
			HttpService: &envoyauth.HttpService{
				ServerUri: httpURI,
				// Trim suffix, as request path always starts with /, and we want to avoid a double /
				PathPrefix:            strings.TrimSuffix(httpService.GetPathPrefix(), "/"),
				AuthorizationRequest:  translateRequest(httpService.GetRequest()),
				AuthorizationResponse: translateResponse(httpService.GetResponse()),
			},
		}
	}

	cfg.FailureModeAllow = settings.GetFailureModeAllow()
	cfg.WithRequestBody = translateRequestBody(settings.GetRequestBody())
	cfg.ClearRouteCache = settings.GetClearRouteCache()
	cfg.StatPrefix = settings.GetStatPrefix()

	statusOnError, err := translateStatusOnError(settings.GetStatusOnError())
	if err != nil {
		return nil, err
	}
	cfg.StatusOnError = statusOnError

	// If not set, `TransportApiVersion` defaults to AUTO (which defaults to V2).
	// Both the AUTO and V2 values are [currently deprecated](https://github.com/envoyproxy/envoy/blob/main/api/envoy/config/core/v3/config_source.proto#L33).
	// These fields will be removed in Envoy [by end of Q1 2021](https://www.envoyproxy.io/docs/envoy/latest/faq/api/envoy_v2_support),
	// when V3 will become the default.
	switch settings.GetTransportApiVersion() {
	case extauthv1.Settings_V3:
		cfg.TransportApiVersion = envoycore.ApiVersion_V3
	default:
		// Leave unset so it defaults to AUTO
	}

	return cfg, nil
}

func translateRequest(in *extauthv1.HttpService_Request) *envoyauth.AuthorizationRequest {
	if in == nil {
		return nil
	}

	return &envoyauth.AuthorizationRequest{
		AllowedHeaders: combineListStringMatchers(
			translateListMatcher(in.GetAllowedHeaders()),
			translateListMatcherRegex(in.GetAllowedHeadersRegex())),
		HeadersToAdd: convertHeadersToAdd(in.GetHeadersToAdd()),
	}
}

func translateResponse(in *extauthv1.HttpService_Response) *envoyauth.AuthorizationResponse {
	if in == nil {
		return nil
	}

	return &envoyauth.AuthorizationResponse{
		AllowedUpstreamHeaders:         translateListMatcher(in.GetAllowedUpstreamHeaders()),
		AllowedClientHeaders:           translateListMatcher(in.GetAllowedClientHeaders()),
		AllowedUpstreamHeadersToAppend: translateListMatcher(in.GetAllowedUpstreamHeadersToAppend()),
	}
}

func translateRequestBody(in *extauthv1.BufferSettings) *envoyauth.BufferSettings {
	if in == nil {
		return nil
	}
	maxBytes := in.GetMaxRequestBytes()
	if maxBytes <= 0 {
		maxBytes = 4 * 1024
	}
	return &envoyauth.BufferSettings{
		AllowPartialMessage: in.GetAllowPartialMessage(),
		MaxRequestBytes:     maxBytes,
		PackAsBytes:         in.GetPackAsBytes(),
	}
}

func translateStatusOnError(statusOnError uint32) (*envoytype.HttpStatus, error) {
	if statusOnError == 0 {
		return nil, nil
	}

	// make sure it is allowed:
	if _, ok := envoytype.StatusCode_name[int32(statusOnError)]; !ok {
		return nil, InvalidStatusOnErrorErr(statusOnError)
	}

	return &envoytype.HttpStatus{Code: envoytype.StatusCode(int32(statusOnError))}, nil
}

func translateListMatcher(in []string) *envoymatcher.ListStringMatcher {
	if len(in) == 0 {
		return nil
	}
	var lsm envoymatcher.ListStringMatcher

	for _, pattern := range in {
		lsm.Patterns = append(lsm.GetPatterns(), &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_Exact{
				Exact: pattern,
			},
		})
	}

	return &lsm
}

func translateListMatcherRegex(in []string) *envoymatcher.ListStringMatcher {
	if len(in) == 0 {
		return nil
	}
	var lsm envoymatcher.ListStringMatcher

	for _, pattern := range in {
		lsm.Patterns = append(lsm.GetPatterns(), &envoymatcher.StringMatcher{
			MatchPattern: &envoymatcher.StringMatcher_SafeRegex{
				SafeRegex: regexutils.NewRegex(context.Background(), pattern),
			},
		})
	}

	return &lsm
}

func combineListStringMatchers(lsms ...*envoymatcher.ListStringMatcher) *envoymatcher.ListStringMatcher {
	var outLSM envoymatcher.ListStringMatcher
	for _, inLSM := range lsms {
		if inLSM != nil {
			outLSM.Patterns = append(outLSM.GetPatterns(), inLSM.GetPatterns()...)
		}
	}
	return &outLSM
}

func convertHeadersToAdd(headersToAddMap map[string]string) []*envoycore.HeaderValue {
	var headersToAdd []*envoycore.HeaderValue
	for k, v := range headersToAddMap {
		headersToAdd = append(headersToAdd, &envoycore.HeaderValue{
			Key:   k,
			Value: v,
		})
	}
	return headersToAdd
}
