# The imported MySQL connection, pointed at credentials Terraform manages. The
# connection is updated in place.

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

resource "montecarlo_warehouse" "mysql" {
  name          = "production-mysql"
  type          = "mysql"
  deployment_id = "<deployment id>"
}

# Pass it as TF_VAR_mysql_password. Ephemeral, so it is never stored in state
# or a plan.
variable "mysql_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "montecarlo_mysql_credentials" "mysql" {
  host = "mysql.example.com"
  port = 3306
  user = "monte_carlo"

  # Changing the password alone plans nothing: bump the version with it.
  password_wo         = var.mysql_password
  password_wo_version = 1
}

resource "montecarlo_connection" "mysql" {
  name           = "production-mysql"
  warehouse_id   = montecarlo_warehouse.mysql.id
  credentials_id = montecarlo_mysql_credentials.mysql.id
}
