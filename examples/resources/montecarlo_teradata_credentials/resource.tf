# A Teradata connection with credentials Monte Carlo stores: the warehouse, a
# database user and its password, and the connection that joins them.
#
# The warehouse type is taken from `connection_type`, and is `teradata` too.
#
# To keep the password in your own store instead, use one of the self-hosted
# credentials resources.
#
# See https://docs.getmontecarlo.com/docs/teradata for creating the user and
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
resource "montecarlo_warehouse" "teradata" {
  name            = "production-teradata"
  connection_type = "teradata"
  deployment_id   = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_teradata_credentials" "teradata" {
  host = "teradata.example.com"
  port = 1025
  user = "monte_carlo"

  # In a real configuration, pass the password from a sensitive variable or a
  # secret store rather than a literal in a committed file.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans
  # nothing: bump the version with it.
  password_wo         = "..."
  password_wo_version = 1

  # `td_logmech` and `td_sslmode` take Teradata's own values. Omit them for the
  # defaults.
  # td_sslmode  = "VERIFY-CA"
  # ssl_ca_data = file("${path.module}/teradata-ca.pem")
}

# The connection's type comes from the credentials, and has to fit the
# warehouse's type.
resource "montecarlo_connection" "teradata" {
  name           = "production-teradata"
  warehouse_id   = montecarlo_warehouse.teradata.id
  credentials_id = montecarlo_teradata_credentials.teradata.id
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.teradata.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.teradata.connection_type
}
