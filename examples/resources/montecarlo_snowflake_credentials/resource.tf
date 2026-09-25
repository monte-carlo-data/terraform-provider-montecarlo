# Snowflake credentials stored by Monte Carlo.
#
# Credentials are a resource of their own, and they do nothing until a connection reads with
# them — see ../montecarlo_connection/resource.tf for the whole flow. They are also independent
# of any deployment: the same credentials can back a connection on any warehouse whose type
# fits them.
#
# This is the Monte Carlo managed option, where Monte Carlo holds the key pair. To keep the
# secret in your own store instead, use one of the self-hosted credentials resources —
# ../montecarlo_self_hosted_aws_credentials/resource.tf is the AWS Secrets Manager one.
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
  # ../../provider/provider.tf for the alternatives.
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

output "credentials_id" {
  description = "Id to give a connection's `credentials_id`."
  value       = montecarlo_snowflake_credentials.snowflake.id
}

output "storage_type" {
  description = "Where the secret lives. mc_managed for these, since Monte Carlo stores them."
  value       = montecarlo_snowflake_credentials.snowflake.storage_type
}
