# A MySQL connection with credentials Monte Carlo stores: the warehouse, a
# database user and its password, and the connection that joins them.
#
# This is the native MySQL connection, on a warehouse of type `mysql`, not a
# generic `transactional-db` one.
#
# To keep the password in your own store instead, use one of the self-hosted
# credentials resources.
#
# See https://docs.getmontecarlo.com/docs/mysql for creating the user and
# granting it the reads Monte Carlo needs.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
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

# The `montecarlo_warehouse` example shows the deployment and agent behind
# `deployment_id`.
resource "montecarlo_warehouse" "mysql" {
  name          = "production-mysql"
  type          = "mysql"
  deployment_id = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_mysql_credentials" "mysql" {
  host = "mysql.example.com"
  port = 3306
  user = "monte_carlo"

  # In a real configuration, pass the password from a sensitive variable or a
  # secret store rather than a literal in a committed file.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans
  # nothing: bump the version with it.
  password_wo         = "..."
  password_wo_version = 1

  # The ssl_* arguments set how the connection uses TLS. Omit them for the
  # defaults.
  # ssl_ca_data = file("${path.module}/mysql-ca.pem")
}

# The connection's type comes from the credentials, and has to fit the
# warehouse's type.
resource "montecarlo_connection" "mysql" {
  name           = "production-mysql"
  warehouse_id   = montecarlo_warehouse.mysql.id
  credentials_id = montecarlo_mysql_credentials.mysql.id
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.mysql.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.mysql.connection_type
}
