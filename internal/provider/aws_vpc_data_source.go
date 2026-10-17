package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Fields match terraform-null-tagger's `network` category.
type awsVPCDataSource struct{}

func NewAWSVPCDataSource() datasource.DataSource {
	return &awsVPCDataSource{}
}

type awsVPCDataSourceModel struct {
	VpcID           types.String `tfsdk:"vpc_id"`
	CidrBlock       types.String `tfsdk:"cidr_block"`
	Ipv6CidrBlock   types.String `tfsdk:"ipv6_cidr_block"`
	InstanceTenancy types.String `tfsdk:"instance_tenancy"`
}

func (d *awsVPCDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_vpc"
}

func (d *awsVPCDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "VPC metadata for terraform-null-tagger's network variable.",
		Attributes: map[string]schema.Attribute{
			"vpc_id": schema.StringAttribute{
				Required:    true,
				Description: "VPC ID, e.g. vpc-0123456789abcdef0.",
			},
			"cidr_block": schema.StringAttribute{
				Computed:    true,
				Description: "Primary IPv4 CIDR block.",
			},
			"ipv6_cidr_block": schema.StringAttribute{
				Computed:    true,
				Description: "IPv6 CIDR block. Empty string if the VPC has no IPv6 association.",
			},
			"instance_tenancy": schema.StringAttribute{
				Computed:    true,
				Description: "default or dedicated. Dedicated VPCs cost more - useful for cost filtering.",
			},
		},
	}
}

func (d *awsVPCDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsVPCDataSourceModel
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
	vpcID := data.VpcID.ValueString()

	out, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		VpcIds: []string{vpcID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe VPC", err.Error())
		return
	}
	if len(out.Vpcs) == 0 {
		resp.Diagnostics.AddError("VPC not found", "No VPC with ID "+vpcID)
		return
	}

	vpc := out.Vpcs[0]

	data.CidrBlock = types.StringValue(derefStr(vpc.CidrBlock))
	data.InstanceTenancy = types.StringValue(string(vpc.InstanceTenancy))

	ipv6 := ""
	for _, assoc := range vpc.Ipv6CidrBlockAssociationSet {
		if assoc.Ipv6CidrBlockState != nil &&
			string(assoc.Ipv6CidrBlockState.State) == "associated" {
			ipv6 = derefStr(assoc.Ipv6CidrBlock)
			break
		}
	}
	data.Ipv6CidrBlock = types.StringValue(ipv6)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
