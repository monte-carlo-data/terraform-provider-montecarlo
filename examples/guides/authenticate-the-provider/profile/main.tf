# A named profile from ~/.mcd/profiles.ini, written by the Monte Carlo CLI.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  profile = "staging"
}
