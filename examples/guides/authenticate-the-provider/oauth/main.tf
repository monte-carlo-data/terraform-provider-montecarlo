# OAuth client credentials. Keep the values in variables, set from a *.tfvars
# file that isn't committed, never as literals in a .tf file.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  client_id     = var.mcd_client_id
  client_secret = var.mcd_client_secret
  # The instance the client belongs to, such as us1.
  instance = var.mcd_instance
}

variable "mcd_client_id" {
  type = string
}

variable "mcd_client_secret" {
  type      = string
  sensitive = true
}

variable "mcd_instance" {
  type = string
}
