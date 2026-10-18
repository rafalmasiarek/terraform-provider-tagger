package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Fields match terraform-null-tagger's `network` category.
type awsIGWDataSource struct{}

func NewAWSIGWDataSource() datasource.DataSource {
	return &awsIGWDataSource{}
}

type awsIGWDataSourceModel struct {
	InternetGatewayID types.String `tfsdk:"internet_gateway_id"`
	VpcID             types.String `tfsdk:"vpc_id"`
	Attached          types.Bool   `tfsdk:"attached"`
}

func (d *awsIGWDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_igw"
}

func (d *awsIGWDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Internet Gateway metadata for terraform-null-tagger's network variable.",
		Attributes: map[string]schema.Attribute{
			"internet_gateway_id": schema.StringAttribute{
				Required:    true,
				Description: "Internet Gateway ID, e.g. igw-0123456789abcdef0.",
			},
			"vpc_id": schema.StringAttribute{
				Computed:    true,
				Description: "VPC this Internet Gateway is attached to. Empty if detached.",
			},
			"attached": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the gateway is currently attached to a VPC.",
			},
		},
	}
}

func (d *awsIGWDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsIGWDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load AWS configuration", err.Error())
		return
	}

	client := ec2.NewFromConfig(cfg)
	igwID := data.InternetGatewayID.ValueString()

	out, err := client.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
		InternetGatewayIds: []string{igwID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe Internet Gateway", err.Error())
		return
	}
	if len(out.InternetGateways) == 0 {
		resp.Diagnostics.AddError("Internet Gateway not found", "No IGW with ID "+igwID)
		return
	}

	igw := out.InternetGateways[0]

	data.VpcID = types.StringValue("")
	data.Attached = types.BoolValue(false)

	for _, att := range igw.Attachments {
		if att.State == ec2types.AttachmentStatusAttached {
			data.VpcID = types.StringValue(derefStr(att.VpcId))
			data.Attached = types.BoolValue(true)
			break
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
