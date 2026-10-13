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
}

variable "hosted_deployment_id" {
  description = "Id of the deployment Monte Carlo hosts for your account."
  type        = string
}

resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = var.hosted_deployment_id
}

resource "montecarlo_snowflake_credentials" "snowflake" {
  # The account identifier, without the .snowflakecomputing.com suffix.
  account = "xy12345.us-east-1"
  user    = "MONTE_CARLO"

  # PEM text. In a real configuration, read the key from a file kept out of
  # version control, with `file("${path.module}/snowflake_key.p8")`.
  private_key_wo         = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MII...
    -----END PRIVATE KEY-----
  EOT
  private_key_wo_version = 1

  # The virtual warehouse queries run in. Omit it to use the user's default.
  warehouse = "MONTE_CARLO_WH"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_snowflake_credentials.snowflake.id
}
