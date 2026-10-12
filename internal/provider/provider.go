package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// taggerProvider has no configuration of its own - every data source is
// self-contained (reads local filesystem state, cloud SDK calls with
// ambient credentials), so there is nothing to configure at the provider
// block level today. Kept as a struct (not a bare function) because the
// framework's Provider interface requires one, and it gives a place to add
// shared configuration later (e.g. default cloud region) without changing
// every data source's signature.
type taggerProvider struct {
	version string
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
		Description: "Auto-discovery data sources for terraform-null-tagger: module origin/version, and (future) provider-agnostic cloud resource metadata.",
	}
}

func (p *taggerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// No provider-level configuration yet.
}

func (p *taggerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOriginDataSource,
	}
}

func (p *taggerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}
