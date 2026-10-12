package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// taggerProvider holds credentials shared across data sources that need
// them. AWS is deliberately absent here - aws-sdk-go-v2's own
// config.LoadDefaultConfig() already reads the standard env vars/shared
// config files, matching how every other AWS-touching tool in this
// toolchain resolves credentials, so there is nothing for this provider to
// add on top of that.
type taggerProvider struct {
	version string
}

type taggerProviderModel struct {
	CloudflareAPIToken types.String `tfsdk:"cloudflare_api_token"`
	HetznerAPIToken    types.String `tfsdk:"hetzner_api_token"`
}

// taggerProviderData is what Configure() hands to every data source via
// req.ProviderData.
type taggerProviderData struct {
	CloudflareAPIToken string
	HetznerAPIToken    string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &taggerProvider{version: version}
	}
}

func (p *taggerProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "tagger"
	resp.Version = p.version
}

func (p *taggerProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Auto-discovery data sources for terraform-null-tagger: module origin/version, and provider-agnostic cloud resource metadata (database engine/version, compute OS, object storage class, ...).",
		Attributes: map[string]schema.Attribute{
			"cloudflare_api_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Falls back to the CLOUDFLARE_API_TOKEN env var if unset - the same name this toolchain already injects via ci/secrets.yml.",
			},
			"hetzner_api_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Falls back to the HCLOUD_TOKEN env var if unset - the same name Hetzner's own official Terraform provider uses.",
			},
		},
	}
}

func (p *taggerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg taggerProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cloudflareToken := cfg.CloudflareAPIToken.ValueString()
	if cloudflareToken == "" {
		cloudflareToken = os.Getenv("CLOUDFLARE_API_TOKEN")
	}

	hetznerToken := cfg.HetznerAPIToken.ValueString()
	if hetznerToken == "" {
		hetznerToken = os.Getenv("HCLOUD_TOKEN")
	}

	data := &taggerProviderData{
		CloudflareAPIToken: cloudflareToken,
		HetznerAPIToken:    hetznerToken,
	}
	resp.DataSourceData = data
}

func (p *taggerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOriginDataSource,
		NewCloudflareR2DataSource,
		NewAWSRDSDataSource,
		NewAWSEC2DataSource,
		NewAWSElastiCacheDataSource,
		NewAWSLambdaDataSource,
		NewHetznerServerDataSource,
	}
}

func (p *taggerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}
