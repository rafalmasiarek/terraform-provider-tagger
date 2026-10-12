package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field names match terraform-null-tagger's new `serverless` variable
// (runtime, memory_mb, timeout_seconds) - added for the ECS/GitHub-runner
// webhook+scale-up Lambda this session's architecture work anticipates.
type awsLambdaDataSource struct{}

func NewAWSLambdaDataSource() datasource.DataSource {
	return &awsLambdaDataSource{}
}

type awsLambdaDataSourceModel struct {
	FunctionName   types.String `tfsdk:"function_name"`
	Runtime        types.String `tfsdk:"runtime"`
	MemoryMB       types.Int64  `tfsdk:"memory_mb"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
}

func (d *awsLambdaDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_lambda"
}

func (d *awsLambdaDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lambda function metadata, for terraform-null-tagger's serverless variable.",
		Attributes: map[string]schema.Attribute{
			"function_name":   schema.StringAttribute{Required: true, Description: "Lambda function name or ARN."},
			"runtime":         schema.StringAttribute{Computed: true, Description: "e.g. nodejs20.x, python3.12."},
			"memory_mb":       schema.Int64Attribute{Computed: true},
			"timeout_seconds": schema.Int64Attribute{Computed: true},
		},
	}
}

func (d *awsLambdaDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsLambdaDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load AWS configuration", err.Error())
		return
	}

	client := lambda.NewFromConfig(cfg)
	functionName := data.FunctionName.ValueString()

	out, err := client.GetFunctionConfiguration(ctx, &lambda.GetFunctionConfigurationInput{
		FunctionName: &functionName,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to get Lambda function configuration", err.Error())
		return
	}

	data.Runtime = types.StringValue(string(out.Runtime))
	if out.MemorySize != nil {
		data.MemoryMB = types.Int64Value(int64(*out.MemorySize))
	}
	if out.Timeout != nil {
		data.TimeoutSeconds = types.Int64Value(int64(*out.Timeout))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
