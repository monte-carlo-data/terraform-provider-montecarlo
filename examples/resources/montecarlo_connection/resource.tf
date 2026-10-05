# A Snowflake connection, end to end: the deployment, the agent, the warehouse that holds the
# connection, the credentials it reads with, and the connection itself.
#
# The chain is deployment -> agent -> warehouse -> credentials -> connection, and Terraform
# derives that order from the references between the blocks. The two halves that meet at the
# connection are independent of each other: a warehouse says which deployment queries run
# through, credentials say what to authenticate with, and neither knows about the other until
# the connection joins them.
#
# The connection's type is not configured — it is taken from the credentials, and it has to fit
# the warehouse's type. Snowflake credentials on a `snowflake` warehouse, as below.
#
# For the Monte Carlo managed credentials used here, see
# the `montecarlo_snowflake_credentials` example. To keep the secret in your own store
# instead, swap that resource for one of the self-hosted ones and point `credentials_id` at it
# — the `montecarlo_self_hosted_aws_credentials` example is the AWS Secrets Manager version,
# and nothing else in this file changes.

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

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AWS"
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws
# The module configures the AWS provider itself, from its own `region` variable.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/aws"
  version = "~> 1.0"

  region = "us-east-1"

  # The external id the deployment generated. This is the dependency that forces the ordering.
  external_id = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}

# `depends_on` rather than a reference: the warehouse needs only the deployment's id, but a
# warehouse whose agent is not yet registered has nothing to serve its connections.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = montecarlo_deployment.agent.id

  depends_on = [montecarlo_aws_collection_agent.agent]
}

resource "montecarlo_snowflake_credentials" "snowflake" {
  account = "xy12345.us-east-1"
  user    = "MONTE_CARLO"

  # PEM text, BEGIN and END lines included. The placeholder below shows the shape; in a real
  # configuration read the key from a file kept out of version control, with
  # `file("${path.module}/snowflake_key.p8")`, rather than pasting it into a .tf file that
  # gets committed. Write-only, so it is never stored in state; bump the version to rotate it.
  private_key_wo         = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MII...
    -----END PRIVATE KEY-----
  EOT
  private_key_wo_version = 1

  warehouse = "MONTE_CARLO_WH"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_snowflake_credentials.snowflake.id

  # The jobs Monte Carlo runs on the connection. Omitting it takes the defaults for the
  # connection type, which is what the Monte Carlo app does and what most configurations want.
  # job_types = ["metadata", "query_logs", "sql_query"]
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.snowflake.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials rather than configured."
  value       = montecarlo_connection.snowflake.connection_type
}

output "credentials_storage_type" {
  description = "Where the secret lives. mc_managed here; a self-hosted credentials resource would report its own store."
  value       = montecarlo_connection.snowflake.credentials_storage_type
}

output "job_types" {
  description = "The jobs Monte Carlo runs on the connection, defaulted by the API when not configured."
  value       = montecarlo_connection.snowflake.job_types
}
