package provider

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Field names match terraform-null-tagger's `cache` variable exactly
// (engine, engine_version, size).
type awsElastiCacheDataSource struct{}

func NewAWSElastiCacheDataSource() datasource.DataSource {
	return &awsElastiCacheDataSource{}
}

type awsElastiCacheDataSourceModel struct {
	CacheClusterID types.String `tfsdk:"cache_cluster_id"`
	Engine         types.String `tfsdk:"engine"`
	EngineVersion  types.String `tfsdk:"engine_version"`
	Size           types.String `tfsdk:"size"`
}

func (d *awsElastiCacheDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aws_elasticache"
}

func (d *awsElastiCacheDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "ElastiCache cluster metadata, for terraform-null-tagger's cache variable.",
		Attributes: map[string]schema.Attribute{
			"cache_cluster_id": schema.StringAttribute{Required: true, Description: "ElastiCache cluster identifier."},
			"engine":           schema.StringAttribute{Computed: true, Description: "redis, memcached"},
			"engine_version":   schema.StringAttribute{Computed: true},
			"size":             schema.StringAttribute{Computed: true, Description: "Cache node type, e.g. cache.t3.micro."},
		},
	}
}

func (d *awsElastiCacheDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data awsElastiCacheDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to load AWS configuration", err.Error())
		return
	}

	client := elasticache.NewFromConfig(cfg)
	clusterID := data.CacheClusterID.ValueString()

	out, err := client.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
		CacheClusterId: &clusterID,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to describe ElastiCache cluster", err.Error())
		return
	}
	if len(out.CacheClusters) == 0 {
		resp.Diagnostics.AddError("ElastiCache cluster not found", "DescribeCacheClusters returned no results for "+clusterID)
		return
	}

	cluster := out.CacheClusters[0]

	data.Engine = types.StringPointerValue(cluster.Engine)
	data.EngineVersion = types.StringPointerValue(cluster.EngineVersion)
	data.Size = types.StringPointerValue(cluster.CacheNodeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
