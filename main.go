package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/monte-carlo-data/terraform-provider-montecarlo/internal/provider"
)

// Regenerates the registry documentation under docs/ from the schemas, examples/ and templates/.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name montecarlo --rendered-provider-name "Monte Carlo"

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
