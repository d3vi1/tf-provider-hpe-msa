package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure the provider satisfies the expected interface.
var _ provider.Provider = (*msaProvider)(nil)

// New returns a new provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &msaProvider{version: version}
	}
}

type msaProvider struct {
	version string
}

type providerConfig struct {
	Endpoint    types.String `tfsdk:"endpoint"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	InsecureTLS types.Bool   `tfsdk:"insecure_tls"`
	Timeout     types.String `tfsdk:"timeout"`
}

type resolvedConfig struct {
	Endpoint    string
	Username    string
	Password    string
	InsecureTLS bool
	Timeout     time.Duration
}

func (p *msaProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "hpe"
	resp.Version = p.version
}

func (p *msaProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "Array HTTPS endpoint (e.g., https://msa.example.com).",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Array username.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Array password.",
				Optional:    true,
				Sensitive:   true,
			},
			"insecure_tls": schema.BoolAttribute{
				Description: "Skip TLS certificate verification (not recommended).",
				Optional:    true,
			},
			"timeout": schema.StringAttribute{
				Description: "HTTP client timeout (e.g., 30s).",
				Optional:    true,
			},
		},
	}
}

func (p *msaProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerConfig
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resolved, diags := resolveConfig(config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := msa.NewClient(msa.Config{
		Endpoint:    resolved.Endpoint,
		Username:    resolved.Username,
		Password:    resolved.Password,
		InsecureTLS: resolved.InsecureTLS,
		Timeout:     resolved.Timeout,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create MSA client", err.Error())
		return
	}

	if resolved.InsecureTLS {
		tflog.Warn(ctx, "TLS certificate verification is disabled")
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *msaProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVolumeResource,
		NewSnapshotResource,
		NewCloneResource,
		NewInitiatorResource,
		NewHostGroupResource,
		NewHostResource,
		NewHostInitiatorResource,
		NewVolumeMappingResource,
	}
}

func (p *msaProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPoolDataSource,
		NewHostDataSource,
		NewVolumeDataSource,
	}
}

func resolveConfig(config providerConfig) (resolvedConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	endpoint, d := stringOrEnv(config.Endpoint, "MSA_ENDPOINT")
	diags.Append(d...)
	username, d := stringOrEnv(config.Username, "MSA_USERNAME")
	diags.Append(d...)
	password, d := stringOrEnv(config.Password, "MSA_PASSWORD")
	diags.Append(d...)
	insecureTLS, d := boolOrEnv(config.InsecureTLS, "MSA_INSECURE_TLS")
	diags.Append(d...)

	var timeout time.Duration
	if config.Timeout.IsUnknown() {
		diags.AddError("Invalid timeout", "timeout is unknown")
	} else if config.Timeout.IsNull() {
		timeout = 30 * time.Second
	} else {
		value, err := time.ParseDuration(config.Timeout.ValueString())
		if err != nil {
			diags.AddError("Invalid timeout", fmt.Sprintf("%q is not a valid duration", config.Timeout.ValueString()))
		} else {
			timeout = value
		}
	}

	if endpoint == "" {
		diags.AddError("Missing endpoint", "Set endpoint in the provider configuration or MSA_ENDPOINT environment variable")
	}
	if username == "" {
		diags.AddError("Missing username", "Set username in the provider configuration or MSA_USERNAME environment variable")
	}
	if password == "" {
		diags.AddError("Missing password", "Set password in the provider configuration or MSA_PASSWORD environment variable")
	}

	return resolvedConfig{
		Endpoint:    endpoint,
		Username:    username,
		Password:    password,
		InsecureTLS: insecureTLS,
		Timeout:     timeout,
	}, diags
}
