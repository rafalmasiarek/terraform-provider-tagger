package provider

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Fields match terraform-null-tagger's `network` category.
// egress_type: derived from the 0.0.0.0/0 route target in the subnet's route table.
type awsSubnetDataSource struct{}

func NewAWSSubnetDataSource() datasource.DataSource {
	return &awsSubnetDataSource{}
}

type awsSubnetDataSourceModel struct {
	SubnetID           types.String `tfsdk:"subnet_id"`
	VpcID              types.String `tfsdk:"vpc_id"`
	AvailabilityZoneID types.String `tfsdk:"availability_zone_id"`
	RouteTableID       types.String `tfsdk:"route_table_id"`
	DefaultForAZ       types.Bool   `tfsdk:"default_for_az"`
	EgressType         types.String `tfsdk:"egress_type"`
}

func (d *awsSubnetDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_subnet"
}

func (d *awsSubnetDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Subnet metadata for terraform-null-tagger's network variable.",
		Attributes: map[string]schema.Attribute{
			"subnet_id": schema.StringAttribute{
				Required:    true,
				Description: "Subnet ID, e.g. subnet-0123456789abcdef0.",
			},
			"vpc_id": schema.StringAttribute{
				Computed:    true,
				Description: "VPC this subnet belongs to.",
			},
			"availability_zone_id": schema.StringAttribute{
				Computed:    true,
				Description: "Stable AZ identifier, e.g. euc1-az3. Unlike the AZ name (eu-central-1a), this never changes.",
			},
			"route_table_id": schema.StringAttribute{
				Computed:    true,
				Description: "Route table associated with this subnet.",
			},
			"default_for_az": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether this is the default subnet for its AZ (gets default SG and RT behavior).",
			},
			"egress_type": schema.StringAttribute{
				Computed:    true,
				Description: "Derived from the 0.0.0.0/0 route: internet-gateway, nat-gateway, network-interface, or none.",
			},
		},
	}
}

func (d *awsSubnetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsSubnetDataSourceModel
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
	subnetID := data.SubnetID.ValueString()

	subnetsOut, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []string{subnetID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe subnet", err.Error())
		return
	}
	if len(subnetsOut.Subnets) == 0 {
		resp.Diagnostics.AddError("Subnet not found", "No subnet with ID "+subnetID)
		return
	}

	subnet := subnetsOut.Subnets[0]

	data.VpcID = types.StringValue(derefStr(subnet.VpcId))
	data.AvailabilityZoneID = types.StringValue(derefStr(subnet.AvailabilityZoneId))
	data.DefaultForAZ = types.BoolValue(derefBool(subnet.DefaultForAz))

	// Find the associated route table via DescribeRouteTables filter
	rtID := ""
	rtsOut, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("association.subnet-id"),
				Values: []string{subnetID},
			},
		},
	})
	if err == nil {
		for _, rt := range rtsOut.RouteTables {
			for _, assoc := range rt.Associations {
				if assoc.SubnetId != nil && *assoc.SubnetId == subnetID &&
					assoc.AssociationState != nil &&
					string(assoc.AssociationState.State) == "associated" {
					rtID = derefStr(rt.RouteTableId)
					break
				}
			}
			if rtID != "" {
				break
			}
		}
	}
	data.RouteTableID = types.StringValue(rtID)

	// Derive egress type from the route table's 0.0.0.0/0 route
	data.EgressType = types.StringValue(resolveEgressType(ctx, client, rtID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func resolveEgressType(ctx context.Context, client *ec2.Client, rtID string) string {
	if rtID == "" {
		return "none"
	}

	rtsOut, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		RouteTableIds: []string{rtID},
	})
	if err != nil || len(rtsOut.RouteTables) == 0 {
		return "none"
	}

	for _, route := range rtsOut.RouteTables[0].Routes {
		if route.DestinationCidrBlock != nil && *route.DestinationCidrBlock == "0.0.0.0/0" {
			switch {
			case route.GatewayId != nil && strings.HasPrefix(*route.GatewayId, "igw-"):
				return "internet-gateway"
			case route.NatGatewayId != nil:
				return "nat-gateway"
			case route.NetworkInterfaceId != nil:
				return "network-interface"
			default:
				return "none"
			}
		}
	}

	return "none"
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
