# A PostgreSQL connection with credentials Monte Carlo stores: the warehouse, a
# database user and its password, and the connection that joins them.
#
# The warehouse type is taken from `connection_type`: PostgreSQL connections
# live on a `transactional-db` warehouse.
#
# To keep the password in your own store instead, use one of the self-hosted
# credentials resources.
#
# See https://docs.getmontecarlo.com/docs/postgres for creating the user and
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
resource "montecarlo_warehouse" "postgres" {
  name            = "production-postgres"
  connection_type = "postgres"
  deployment_id   = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_postgres_credentials" "postgres" {
  host    = "postgres.example.com"
  port    = 5432
  db_name = "analytics"
  user    = "monte_carlo"

  # In a real configuration, pass the password from a sensitive variable or a
  # secret store rather than a literal in a committed file.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans
  # nothing: bump the version with it.
  password_wo         = "..."
  password_wo_version = 1

  # The ssl_* arguments set how the connection uses TLS. Omit them for the
  # defaults. Set `rds_proxy = true` when `host` is an Amazon RDS Proxy.
  # ssl_verify_cert = true
  # ssl_ca_data     = file("${path.module}/postgres-ca.pem")
}

# The connection's type comes from the credentials, and has to fit the
# warehouse's type.
resource "montecarlo_connection" "postgres" {
  name           = "production-postgres"
  warehouse_id   = montecarlo_warehouse.postgres.id
  credentials_id = montecarlo_postgres_credentials.postgres.id
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.postgres.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.postgres.connection_type
}
