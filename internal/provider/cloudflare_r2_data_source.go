package provider

import (
	"context"
	"fmt"

	cloudflare "github.com/cloudflare/cloudflare-go/v4"
	"github.com/cloudflare/cloudflare-go/v4/option"
	"github.com/cloudflare/cloudflare-go/v4/r2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field names match terraform-null-tagger's `object_storage` variable
// exactly (storage_class, versioning), so wiring one into the other is a
// straight copy, not a translation. R2's Bucket type has no versioning
// field at all (confirmed via `go doc` against the real v4 SDK, not
// assumed from memory) - versioning is left unset here rather than
// invented.
type cloudflareR2DataSource struct {
	providerData *taggerProviderData
}

func NewCloudflareR2DataSource() datasource.DataSource {
	return &cloudflareR2DataSource{}
}

type cloudflareR2DataSourceModel struct {
	AccountID    types.String `tfsdk:"account_id"`
	BucketName   types.String `tfsdk:"bucket_name"`
	StorageClass types.String `tfsdk:"storage_class"`
}

func (d *cloudflareR2DataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloudflare_r2"
}

func (d *cloudflareR2DataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "R2 bucket metadata, for terraform-null-tagger's object_storage variable.",
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				Required:    true,
				Description: "Cloudflare account ID the bucket belongs to.",
			},
			"bucket_name": schema.StringAttribute{
				Required:    true,
				Description: "R2 bucket name.",
			},
			"storage_class": schema.StringAttribute{
				Computed:    true,
				Description: "\"Standard\" or \"InfrequentAccess\" - storage class for newly uploaded objects.",
			},
		},
	}
}

func (d *cloudflareR2DataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*taggerProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", fmt.Sprintf("expected *taggerProviderData, got %T", req.ProviderData))
		return
	}
	d.providerData = data
}

func (d *cloudflareR2DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data cloudflareR2DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.providerData == nil || d.providerData.CloudflareAPIToken == "" {
		resp.Diagnostics.AddError(
			"Missing Cloudflare API token",
			"Set cloudflare_api_token on the tagger provider block, or export CLOUDFLARE_API_TOKEN.",
		)
		return
	}

	bucketService := r2.NewBucketService(option.WithAPIToken(d.providerData.CloudflareAPIToken))

	bucket, err := bucketService.Get(ctx, data.BucketName.ValueString(), r2.BucketGetParams{
		AccountID: cloudflare.F(data.AccountID.ValueString()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to read R2 bucket", err.Error())
		return
	}

	data.StorageClass = types.StringValue(string(bucket.StorageClass))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
