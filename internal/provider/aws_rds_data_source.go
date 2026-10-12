package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// No provider-level AWS credentials here on purpose - aws-sdk-go-v2's own
// config.LoadDefaultConfig() reads the standard env vars/shared config
// files (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/profiles/...), the exact
// same chain every other AWS-touching tool in this toolchain already
// relies on. Field names match terraform-null-tagger's `database`
// variable (engine, engine_version, size, multi_az, storage_type,
// storage_encrypted). storage_gb is deliberately absent - RDS storage
// autoscaling changes AllocatedStorage outside Terraform, so reading it
// here would cause a perpetual plan diff. storage_type is safe to include
// even though it lives next to storage_gb - the type itself never changes
// without an explicit modify-instance call, unlike the allocated size.
type awsRDSDataSource struct{}

func NewAWSRDSDataSource() datasource.DataSource {
	return &awsRDSDataSource{}
}

type awsRDSDataSourceModel struct {
	DBInstanceIdentifier types.String `tfsdk:"db_instance_identifier"`
	Engine               types.String `tfsdk:"engine"`
	EngineVersion        types.String `tfsdk:"engine_version"`
	Size                 types.String `tfsdk:"size"`
	MultiAZ              types.Bool   `tfsdk:"multi_az"`
	StorageType          types.String `tfsdk:"storage_type"`
	StorageEncrypted     types.Bool   `tfsdk:"storage_encrypted"`
}

func (d *awsRDSDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_rds"
}

func (d *awsRDSDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "RDS instance metadata, for terraform-null-tagger's database variable.",
		Attributes: map[string]schema.Attribute{
			"db_instance_identifier": schema.StringAttribute{
				Required:    true,
				Description: "RDS DB instance identifier.",
			},
			"engine":            schema.StringAttribute{Computed: true, Description: "postgres, mysql, mariadb, ..."},
			"engine_version":    schema.StringAttribute{Computed: true},
			"size":              schema.StringAttribute{Computed: true, Description: "DB instance class, e.g. db.t3.micro."},
			"multi_az":          schema.BoolAttribute{Computed: true},
			"storage_type":      schema.StringAttribute{Computed: true, Description: "gp2, gp3, io1, io2, ..."},
			"storage_encrypted": schema.BoolAttribute{Computed: true},
		},
	}
}

func (d *awsRDSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsRDSDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load AWS configuration", err.Error())
		return
	}

	client := rds.NewFromConfig(cfg)
	identifier := data.DBInstanceIdentifier.ValueString()

	out, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &identifier,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe RDS instance", err.Error())
		return
	}
	if len(out.DBInstances) == 0 {
		resp.Diagnostics.AddError("RDS instance not found", "DescribeDBInstances returned no results for "+identifier)
		return
	}

	instance := out.DBInstances[0]

	data.Engine = types.StringPointerValue(instance.Engine)
	data.EngineVersion = types.StringPointerValue(instance.EngineVersion)
	data.Size = types.StringPointerValue(instance.DBInstanceClass)
	if instance.MultiAZ != nil {
		data.MultiAZ = types.BoolValue(*instance.MultiAZ)
	}
	data.StorageType = types.StringPointerValue(instance.StorageType)
	if instance.StorageEncrypted != nil {
		data.StorageEncrypted = types.BoolValue(*instance.StorageEncrypted)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
