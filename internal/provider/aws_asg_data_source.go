package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
)

// Field names match terraform-null-tagger's `autoscaling_group` variable.
// desired_capacity is deliberately absent - the autoscaler itself changes it
// outside of Terraform (target tracking, scheduled actions), so exposing it
// here would cause a perpetual plan diff. min_size/max_size and the launch
// template are configuration, not live state, so they're safe to tag.
type awsASGDataSource struct{}

func NewAWSASGDataSource() datasource.DataSource {
	return &awsASGDataSource{}
}

type awsASGDataSourceModel struct {
	AutoScalingGroupName  tftypes.String `tfsdk:"autoscaling_group_name"`
	MinSize               tftypes.Int64  `tfsdk:"min_size"`
	MaxSize               tftypes.Int64  `tfsdk:"max_size"`
	LaunchTemplateID      tftypes.String `tfsdk:"launch_template_id"`
	LaunchTemplateVersion tftypes.String `tfsdk:"launch_template_version"`
}

func (d *awsASGDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_asg"
}

func (d *awsASGDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Auto Scaling group metadata, for terraform-null-tagger's autoscaling_group variable. Omits desired_capacity on purpose - it's live state, not configuration.",
		Attributes: map[string]schema.Attribute{
			"autoscaling_group_name":  schema.StringAttribute{Required: true},
			"min_size":                schema.Int64Attribute{Computed: true},
			"max_size":                schema.Int64Attribute{Computed: true},
			"launch_template_id":      schema.StringAttribute{Computed: true, Description: "From the ASG's own launch template, or its mixed-instances-policy launch template if set. Empty if the ASG uses a launch configuration instead."},
			"launch_template_version": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *awsASGDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsASGDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load AWS configuration", err.Error())
		return
	}

	client := autoscaling.NewFromConfig(cfg)
	name := data.AutoScalingGroupName.ValueString()

	out, err := client.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{name},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe Auto Scaling group", err.Error())
		return
	}
	if len(out.AutoScalingGroups) == 0 {
		resp.Diagnostics.AddError("Auto Scaling group not found", "DescribeAutoScalingGroups returned no results for "+name)
		return
	}

	asg := out.AutoScalingGroups[0]

	data.MinSize = int32PtrToInt64Value(asg.MinSize)
	data.MaxSize = int32PtrToInt64Value(asg.MaxSize)

	spec := launchTemplateSpec(asg)
	if spec != nil {
		data.LaunchTemplateID = tftypes.StringPointerValue(spec.LaunchTemplateId)
		data.LaunchTemplateVersion = tftypes.StringPointerValue(spec.Version)
	} else {
		data.LaunchTemplateID = tftypes.StringValue("")
		data.LaunchTemplateVersion = tftypes.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// launchTemplateSpec prefers the ASG's own launch template, falling back to
// the mixed-instances-policy one. Returns nil if the ASG uses a launch
// configuration instead of a launch template at all.
func launchTemplateSpec(asg types.AutoScalingGroup) *types.LaunchTemplateSpecification {
	if asg.LaunchTemplate != nil {
		return asg.LaunchTemplate
	}
	if asg.MixedInstancesPolicy != nil && asg.MixedInstancesPolicy.LaunchTemplate != nil {
		return asg.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
	}
	return nil
}

func int32PtrToInt64Value(v *int32) basetypes.Int64Value {
	if v == nil {
		return tftypes.Int64Null()
	}
	return tftypes.Int64Value(int64(*v))
}
