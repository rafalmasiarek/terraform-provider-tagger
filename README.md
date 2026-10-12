# terraform-provider-tagger

Data sources feeding [terraform-null-tagger](https://github.com/rafalmasiarek/terraform-null-tagger).

## Data sources

`tagger_origin` - calling module's own git origin + version. Resolved via
`git remote get-url origin` / `git describe --tags` run in `path.module`,
falling back to `.terraform/modules/modules.json`. Never fails the plan -
returns `"unknown"` on a miss.

Every other data source maps to one `terraform-null-tagger` service
variable. Fields marked `*` are required (the resource identifier);
everything else is computed from the live resource. A field is left out
on purpose when the cloud provider itself can change it outside Terraform
(autoscaling, storage autoscaling, ...) - including it would cause a plan
diff on every apply with no config change behind it.

### AWS

| Data source | Inputs | `terraform-null-tagger` variable |
|---|---|---|
| `tagger_aws_rds` | `db_instance_identifier`\*, `engine`, `engine_version`, `size`, `multi_az` | `database` |
| `tagger_aws_ec2` | `instance_id`\*, `instance_type`, `os` | `compute` |
| `tagger_aws_elasticache` | `cache_cluster_id`\*, `engine`, `engine_version`, `size` | `cache` |
| `tagger_aws_lambda` | `function_name`\*, `runtime`, `memory_mb`, `timeout_seconds` | `serverless` |
| `tagger_aws_asg` | `autoscaling_group_name`\*, `min_size`, `max_size`, `launch_template_id`, `launch_template_version` | `autoscaling_group` |

### Cloudflare

| Data source | Inputs | `terraform-null-tagger` variable |
|---|---|---|
| `tagger_cloudflare_r2` | `account_id`\*, `bucket_name`\*, `storage_class` | `object_storage` |

### Hetzner

| Data source | Inputs | `terraform-null-tagger` variable |
|---|---|---|
| `tagger_hetzner_server` | `server_id`\*, `instance_id`, `instance_type`, `os`, `os_version` | `compute` |

\* - required

## Usage

```hcl
data "tagger_origin" "self" {
  module_path = path.module
}
```

## Author

[Rafał Masiarek](https://github.com/rafalmasiarek)

