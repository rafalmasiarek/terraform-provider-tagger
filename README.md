# terraform-provider-tagger

Data sources feeding [terraform-null-tagger](https://github.com/rafalmasiarek/terraform-null-tagger).

## Data sources

- `tagger_origin` - calling module's own git origin + version. Resolved via
  `git remote get-url origin` / `git describe --tags` run in `path.module`,
  falling back to `.terraform/modules/modules.json`. Never fails the plan -
  returns `"unknown"` on a miss.
- `tagger_cloudflare_r2`, `tagger_aws_rds`, `tagger_aws_ec2`,
  `tagger_aws_elasticache`, `tagger_aws_lambda`, `tagger_hetzner_server` -
  per-resource metadata (storage class, engine, instance type, ...) for the
  matching `terraform-null-tagger` service tag set.

## Usage

```hcl
data "tagger_origin" "self" {
  module_path = path.module
}
```

