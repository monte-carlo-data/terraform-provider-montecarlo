# Snowflake credentials in GCP Secret Manager, read by a GCP collection agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    google = {
      source = "hashicorp/google"
    }
  }
}

provider "google" {
  project = "my-project"
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

variable "agent_service_account_email" {
  description = "Email of the service account the agent's Cloud Run service runs as. The agent module does not output it."
  type        = string
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

locals {
  secret_name = "monte-carlo-snowflake"
}

# The agent module grants no Secret Manager access.
resource "google_secret_manager_secret_iam_member" "agent_read_secret" {
  secret_id = local.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.agent_service_account_email}"
}

resource "montecarlo_self_hosted_gcp_credentials" "snowflake" {
  connection_type = "snowflake"
  gcp_secret      = local.secret_name

  # The grant has to exist before Monte Carlo is told where the secret is.
  depends_on = [google_secret_manager_secret_iam_member.agent_read_secret]
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_gcp_credentials.snowflake.id
}
