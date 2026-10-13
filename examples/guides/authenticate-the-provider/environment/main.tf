# Credentials from MCD_DEFAULT_* environment variables, or from the Monte Carlo
# CLI's profile, so the provider block is empty.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {}
