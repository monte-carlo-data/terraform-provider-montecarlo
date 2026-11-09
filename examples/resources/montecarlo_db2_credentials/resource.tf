# A Db2 connection with credentials Monte Carlo stores: the warehouse, a
# database user and its password, and the connection that joins them.
#
# The warehouse type is taken from `connection_type`, and is `db2` too.
#
# To keep the password in your own store instead, use one of the self-hosted
# credentials resources.
#
# See https://docs.getmontecarlo.com/docs/db2 for creating the user and
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
resource "montecarlo_warehouse" "db2" {
  name            = "production-db2"
  connection_type = "db2"
  deployment_id   = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_db2_credentials" "db2" {
  host    = "db2.example.com"
  port    = 50000
  db_name = "ANALYTICS"
  user    = "monte_carlo"

  # In a real configuration, pass the password from a sensitive variable or a
  # secret store rather than a literal in a committed file.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans
  # nothing: bump the version with it.
  password_wo         = "..."
  password_wo_version = 1

  # The ssl_* arguments set how the connection uses TLS. Omit them for the
  # defaults.
  # ssl_ca_data = file("${path.module}/db2-ca.pem")
}

# The connection's type comes from the credentials, and has to fit the
# warehouse's type.
resource "montecarlo_connection" "db2" {
  name           = "production-db2"
  warehouse_id   = montecarlo_warehouse.db2.id
  credentials_id = montecarlo_db2_credentials.db2.id
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.db2.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.db2.connection_type
}
