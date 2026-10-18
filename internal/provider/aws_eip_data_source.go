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
type awsEIPDataSource struct{}

func NewAWSEIPDataSource() datasource.DataSource {
	return &awsEIPDataSource{}
}

type awsEIPDataSourceModel struct {
	AllocationID       types.String `tfsdk:"allocation_id"`
	Domain             types.String `tfsdk:"domain"`
	PublicIP           types.String `tfsdk:"public_ip"`
	AttachmentType     types.String `tfsdk:"attachment_type"`
	InstanceID         types.String `tfsdk:"instance_id"`
	NetworkInterfaceID types.String `tfsdk:"network_interface_id"`
}

func (d *awsEIPDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_eip"
}

func (d *awsEIPDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "EIP metadata for terraform-null-tagger's network variable.",
		Attributes: map[string]schema.Attribute{
			"allocation_id": schema.StringAttribute{
				Required:    true,
				Description: "EIP allocation ID, e.g. eipalloc-0123456789abcdef0.",
			},
			"domain": schema.StringAttribute{
				Computed:    true,
				Description: "vpc or standard (ec2-classic).",
			},
			"public_ip": schema.StringAttribute{
				Computed:    true,
				Description: "The public IP address.",
			},
			"attachment_type": schema.StringAttribute{
				Computed:    true,
				Description: "Derived: ec2-instance, network-interface, or unassigned.",
			},
			"instance_id": schema.StringAttribute{
				Computed:    true,
				Description: "EC2 instance ID if attached. Empty otherwise.",
			},
			"network_interface_id": schema.StringAttribute{
				Computed:    true,
				Description: "ENI ID if attached to a network interface. Empty otherwise.",
			},
		},
	}
}

func (d *awsEIPDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsEIPDataSourceModel
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
	allocationID := data.AllocationID.ValueString()

	out, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
		AllocationIds: []string{allocationID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe EIP", err.Error())
		return
	}
	if len(out.Addresses) == 0 {
		resp.Diagnostics.AddError("EIP not found", "No EIP with allocation ID "+allocationID)
		return
	}

	addr := out.Addresses[0]

	data.Domain = types.StringValue(string(addr.Domain))
	data.PublicIP = types.StringValue(derefStr(addr.PublicIp))
	data.InstanceID = types.StringValue(derefStr(addr.InstanceId))
	data.NetworkInterfaceID = types.StringValue(derefStr(addr.NetworkInterfaceId))

	data.AttachmentType = types.StringValue(resolveEIPAttachmentType(addr.InstanceId, addr.NetworkInterfaceId))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func resolveEIPAttachmentType(instanceID, eniID *string) string {
	switch {
	case derefStr(instanceID) != "":
		return "ec2-instance"
	case derefStr(eniID) != "":
		return "network-interface"
	default:
		return "unassigned"
	}
}
