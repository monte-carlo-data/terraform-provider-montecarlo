package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/monte-carlo-data/terraform-provider-montecarlo/internal/provider"
)

// version is set at release time. It is reported to Terraform and appears in user agents.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(), providerserver.ServeOpts{
		Address: "registry.terraform.io/monte-carlo-data/montecarlo",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
