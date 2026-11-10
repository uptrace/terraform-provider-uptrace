package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/uptrace/terraform/provider"
	"github.com/uptrace/terraform/version"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "enable debug mode")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version.ProviderVersion), providerserver.ServeOpts{
		Address: "registry.terraform.io/uptrace/uptrace",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
