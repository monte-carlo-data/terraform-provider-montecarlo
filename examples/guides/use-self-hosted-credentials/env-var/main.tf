# Snowflake credentials in an environment variable on the agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

resource "montecarlo_self_hosted_env_var_credentials" "snowflake" {
  connection_type = "snowflake"
  # Must start with MCD_.
  env_var_name = "MCD_SNOWFLAKE_CREDENTIALS"
  # Only when the value is encrypted with an AWS KMS key.
  # kms_key_id = "arn:aws:kms:us-east-1:123456789012:key/..."
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_env_var_credentials.snowflake.id
}
