# A BigQuery connection with credentials Monte Carlo stores: the warehouse, a service account's
# JSON key, and the connection that joins them.
#
# To keep the key in your own store instead, use one of the self-hosted credentials resources.
#
# See https://docs.getmontecarlo.com/docs/bigquery for creating the service account and the
# roles Monte Carlo needs.

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

# ../montecarlo_warehouse/resource.tf shows the deployment and agent behind `deployment_id`.
resource "montecarlo_warehouse" "bigquery" {
  name          = "production-bigquery"
  type          = "bigquery"
  deployment_id = "<deployment id, from the Monte Carlo app>"
}

resource "montecarlo_bigquery_credentials" "bigquery" {
  # The key file's text. The placeholder shows the shape; in a real configuration read the
  # file, kept out of version control, with `file("${path.module}/service_account_key.json")`.
  #
  # Write-only, so never stored in state or a plan. Changing it alone plans nothing: bump the
  # version with it.
  service_account_key_wo         = <<-EOT
    {
      "type": "service_account",
      "project_id": "my-project",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "monte-carlo@my-project.iam.gserviceaccount.com"
    }
  EOT
  service_account_key_wo_version = 1
}

# The connection's type comes from the credentials, and has to fit the warehouse's type.
resource "montecarlo_connection" "bigquery" {
  name           = "production-bigquery"
  warehouse_id   = montecarlo_warehouse.bigquery.id
  credentials_id = montecarlo_bigquery_credentials.bigquery.id
}

output "project_id" {
  description = "Project the service account belongs to, read from the key."
  value       = montecarlo_bigquery_credentials.bigquery.project_id
}

output "client_email" {
  description = "The service account, read from the key."
  value       = montecarlo_bigquery_credentials.bigquery.client_email
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.bigquery.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials."
  value       = montecarlo_connection.bigquery.connection_type
}
