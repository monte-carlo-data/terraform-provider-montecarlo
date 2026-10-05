# A Snowflake connection with credentials Monte Carlo stores: the warehouse, the credentials,
# and the connection that joins them.
#
# This is the Monte Carlo managed option, where Monte Carlo holds the key pair. To keep the
# secret in your own store instead, use one of the self-hosted credentials resources —
# the `montecarlo_self_hosted_aws_credentials` example is the AWS Secrets Manager one.
#
# Authentication is key pair only; there is no password option. See
# https://docs.getmontecarlo.com/docs/snowflake for provisioning the service user and granting
# it the reads Monte Carlo needs.

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
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # the provider documentation for the alternatives.
}

# The `montecarlo_warehouse` example shows the deployment and agent behind `deployment_id`.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_snowflake_credentials" "snowflake" {
  # The account identifier, without the .snowflakecomputing.com suffix.
  account = "xy12345.us-east-1"
  user    = "MONTE_CARLO"

  # PEM text, BEGIN and END lines included. The placeholder below shows the shape; in a real
  # configuration read the key from a file kept out of version control, with
  # `file("${path.module}/snowflake_key.p8")`, rather than pasting it into a .tf file that
  # gets committed.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans nothing: bump the
  # version with it. Bumping either version sends the key and the passphrase together.
  private_key_wo         = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MII...
    -----END PRIVATE KEY-----
  EOT
  private_key_wo_version = 1

  # Only for an encrypted key. Omit both for an unencrypted one.
  # private_key_passphrase_wo         = "..."
  # private_key_passphrase_wo_version = 1

  # The virtual warehouse queries run in. Omit it to use the user's default.
  warehouse = "MONTE_CARLO_WH"
}

# The connection's type comes from the credentials, and has to fit the warehouse's type.
resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_snowflake_credentials.snowflake.id
}

output "storage_type" {
  description = "Where the secret lives. mc_managed for these, since Monte Carlo stores them."
  value       = montecarlo_snowflake_credentials.snowflake.storage_type
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.snowflake.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.snowflake.connection_type
}
