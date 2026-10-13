# Snowflake credentials in Azure Key Vault, read by an Azure collection agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

resource "montecarlo_self_hosted_azure_credentials" "snowflake" {
  connection_type = "snowflake"
  akv_secret      = "monte-carlo-snowflake"
  akv_vault_name  = "my-key-vault"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_azure_credentials.snowflake.id
}
