package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/rafalmasiarek/terraform-provider-tagger/internal/provider"
)

// version is overwritten at build time via -ldflags, e.g.:
//
//	go build -ldflags "-X main.version=0.1.0"
var version = "dev"

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// Local, dev_overrides address - not published to the public
		// registry yet. See .terraformrc / examples/provider-install.tf
		// for how to point a consumer's Terraform CLI at this binary.
		Address: "registry.terraform.io/rafalmasiarek/tagger",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
