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
type awsNatGatewayDataSource struct{}

func NewAWSNatGatewayDataSource() datasource.DataSource {
	return &awsNatGatewayDataSource{}
}

type awsNatGatewayDataSourceModel struct {
	NatGatewayID     types.String `tfsdk:"nat_gateway_id"`
	VpcID            types.String `tfsdk:"vpc_id"`
	SubnetID         types.String `tfsdk:"subnet_id"`
	AvailabilityZone types.String `tfsdk:"availability_zone"`
	AllocationID     types.String `tfsdk:"allocation_id"`
}

func (d *awsNatGatewayDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_nat_gateway"
}

func (d *awsNatGatewayDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "NAT Gateway metadata for terraform-null-tagger's network variable.",
		Attributes: map[string]schema.Attribute{
			"nat_gateway_id": schema.StringAttribute{
				Required:    true,
				Description: "NAT Gateway ID, e.g. nat-0123456789abcdef0.",
			},
			"vpc_id": schema.StringAttribute{
				Computed:    true,
				Description: "VPC this NAT Gateway belongs to (from its subnet).",
			},
			"subnet_id": schema.StringAttribute{
				Computed:    true,
				Description: "Subnet the NAT Gateway resides in.",
			},
			"availability_zone": schema.StringAttribute{
				Computed:    true,
				Description: "Availability Zone of the NAT Gateway's subnet.",
			},
			"allocation_id": schema.StringAttribute{
				Computed:    true,
				Description: "EIP allocation ID associated with this NAT Gateway.",
			},
		},
	}
}

func (d *awsNatGatewayDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsNatGatewayDataSourceModel
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
	natID := data.NatGatewayID.ValueString()

	out, err := client.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []string{natID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe NAT Gateway", err.Error())
		return
	}
	if len(out.NatGateways) == 0 {
		resp.Diagnostics.AddError("NAT Gateway not found", "No NAT Gateway with ID "+natID)
		return
	}

	nat := out.NatGateways[0]
	subnetID := derefStr(nat.SubnetId)
	data.SubnetID = types.StringValue(subnetID)

	allocationID := ""
	if len(nat.NatGatewayAddresses) > 0 {
		allocationID = derefStr(nat.NatGatewayAddresses[0].AllocationId)
	}
	data.AllocationID = types.StringValue(allocationID)

	// VPC and AZ come from the subnet
	data.VpcID = types.StringValue("")
	data.AvailabilityZone = types.StringValue("")
	if subnetID != "" {
		subnetsOut, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: []string{subnetID},
		})
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Could not resolve NAT Gateway subnet",
				"DescribeSubnets failed: "+err.Error(),
			)
		} else if len(subnetsOut.Subnets) > 0 {
			data.VpcID = types.StringValue(derefStr(subnetsOut.Subnets[0].VpcId))
			data.AvailabilityZone = types.StringValue(derefStr(subnetsOut.Subnets[0].AvailabilityZone))
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
