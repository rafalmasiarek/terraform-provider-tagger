terraform {
  required_providers {
    tagger = {
      source = "registry.terraform.io/rafalmasiarek/tagger"
    }
  }
}

data "tagger_origin" "self" {
  module_path = path.module
}

output "source" {
  value = data.tagger_origin.self.source
}

output "version" {
  value = data.tagger_origin.self.version
}
