module "inner" {
  source = "./inner"
}

output "source" {
  value = module.inner.source
}

output "version" {
  value = module.inner.version
}
