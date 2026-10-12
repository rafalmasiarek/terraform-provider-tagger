package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// Field names match terraform-null-tagger's `compute` variable exactly
// (instance_id, instance_type, os, os_version).
type hetznerServerDataSource struct {
	providerData *taggerProviderData
}

func NewHetznerServerDataSource() datasource.DataSource {
	return &hetznerServerDataSource{}
}

type hetznerServerDataSourceModel struct {
	ServerID     types.Int64  `tfsdk:"server_id"`
	InstanceID   types.String `tfsdk:"instance_id"`
	InstanceType types.String `tfsdk:"instance_type"`
	OS           types.String `tfsdk:"os"`
	OSVersion    types.String `tfsdk:"os_version"`
}

func (d *hetznerServerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hetzner_server"
}

func (d *hetznerServerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Hetzner Cloud server metadata, for terraform-null-tagger's compute variable.",
		Attributes: map[string]schema.Attribute{
			"server_id":     schema.Int64Attribute{Required: true, Description: "Hetzner Cloud server ID."},
			"instance_id":   schema.StringAttribute{Computed: true, Description: "server_id as a string, matching compute.instance_id's type."},
			"instance_type": schema.StringAttribute{Computed: true, Description: "Server type name, e.g. cx22."},
			"os":            schema.StringAttribute{Computed: true, Description: "Image OS flavor, e.g. ubuntu, debian."},
			"os_version":    schema.StringAttribute{Computed: true},
		},
	}
}

func (d *hetznerServerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *hetznerServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data hetznerServerDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.providerData == nil || d.providerData.HetznerAPIToken == "" {
		resp.Diagnostics.AddError(
			"Missing Hetzner API token",
			"Set hetzner_api_token on the tagger provider block, or export HCLOUD_TOKEN.",
		)
		return
	}

	client := hcloud.NewClient(hcloud.WithToken(d.providerData.HetznerAPIToken))

	server, _, err := client.Server.GetByID(ctx, data.ServerID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read Hetzner server", err.Error())
		return
	}
	if server == nil {
		resp.Diagnostics.AddError("Hetzner server not found", "GetByID returned no server for ID "+strconv.FormatInt(data.ServerID.ValueInt64(), 10))
		return
	}

	data.InstanceID = types.StringValue(strconv.FormatInt(server.ID, 10))
	if server.ServerType != nil {
		data.InstanceType = types.StringValue(server.ServerType.Name)
	}
	if server.Image != nil {
		data.OS = types.StringValue(server.Image.OSFlavor)
		data.OSVersion = types.StringValue(server.Image.OSVersion)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
