package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field names match terraform-null-tagger's `compute` variable exactly
// (instance_id, instance_type, os, os_version). os_version is
// deliberately left unset - DescribeInstances has no reliable OS version
// field without a separate AMI/SSM lookup, and this module does not
// invent data it can't actually source.
type awsEC2DataSource struct{}

func NewAWSEC2DataSource() datasource.DataSource {
	return &awsEC2DataSource{}
}

type awsEC2DataSourceModel struct {
	InstanceID   types.String `tfsdk:"instance_id"`
	InstanceType types.String `tfsdk:"instance_type"`
	OS           types.String `tfsdk:"os"`
}

func (d *awsEC2DataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_ec2"
}

func (d *awsEC2DataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "EC2 instance metadata, for terraform-null-tagger's compute variable.",
		Attributes: map[string]schema.Attribute{
			"instance_id":   schema.StringAttribute{Required: true, Description: "EC2 instance ID, e.g. i-0123456789abcdef0."},
			"instance_type": schema.StringAttribute{Computed: true, Description: "e.g. t3.micro."},
			"os":            schema.StringAttribute{Computed: true, Description: "Platform details, e.g. \"Linux/UNIX\", \"Red Hat Enterprise Linux\"."},
		},
	}
}

func (d *awsEC2DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsEC2DataSourceModel
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
	instanceID := data.InstanceID.ValueString()

	out, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe EC2 instance", err.Error())
		return
	}
	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		resp.Diagnostics.AddError("EC2 instance not found", "DescribeInstances returned no results for "+instanceID)
		return
	}

	instance := out.Reservations[0].Instances[0]

	data.InstanceType = types.StringValue(string(instance.InstanceType))
	data.OS = types.StringPointerValue(instance.PlatformDetails)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
