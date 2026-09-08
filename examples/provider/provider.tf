terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

# Credentials may be set here, taken from the environment, or read from a profile written by the
# Monte Carlo CLI, in that order. Keep them in variables, with the values in a *.tfvars file
# that is not committed — never as literals in a .tf file.
provider "montecarlo" {
  endpoint     = "https://api.getmontecarlo.com"
  token_id     = var.mcd_id
  token_secret = var.mcd_token
}

# If you have run the Monte Carlo CLI, all three can be left out: the endpoint and the token
# come from ~/.mcd/profiles.ini, and this is the whole provider block.
#
#   provider "montecarlo" {}

variable "mcd_id" {
  description = "Monte Carlo API token id. Falls back to MCD_DEFAULT_API_ID, then to the profile's mcd_id."
  type        = string
  default     = null
}

variable "mcd_token" {
  description = "Monte Carlo API token secret. Falls back to MCD_DEFAULT_API_TOKEN, then to the profile's mcd_token."
  type        = string
  sensitive   = true
  default     = null
}
