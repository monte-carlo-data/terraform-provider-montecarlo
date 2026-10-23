# Bring a MySQL warehouse and connection created in the Monte Carlo app under
# Terraform, unchanged. The first plan imports both and changes nothing.

terraform {
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

import {
  to = montecarlo_warehouse.mysql
  id = "<warehouse id>"
}

import {
  to = montecarlo_connection.mysql
  id = "<connection id>"
}

# `type`, not `connection_type`: import leaves `connection_type` unset, and
# setting it replaces the warehouse.
resource "montecarlo_warehouse" "mysql" {
  name          = "production-mysql"
  type          = "mysql"
  deployment_id = "<deployment id>"
}

# No `credentials_id` yet: a connection created in the app has none.
resource "montecarlo_connection" "mysql" {
  name         = "production-mysql"
  warehouse_id = montecarlo_warehouse.mysql.id
}
