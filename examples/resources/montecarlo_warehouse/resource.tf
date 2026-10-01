# A warehouse, with the deployment and agent its connections run through.
#
# A warehouse is a container: it holds connections, and on its own it queries nothing. Its
# `deployment_id` decides which collection agent those connections run through, so the agent
# has to be registered and working before a connection added here can reach anything. For the
# rest of the flow — credentials, and the connection itself — see
# the `montecarlo_connection` example.
#
# `type` and `connection_type` are alternatives and the API refuses both together. `type` names
# the warehouse type outright, as below. `connection_type` instead names the type of the first
# connection you intend to add, and Monte Carlo derives the warehouse type from it.

terraform {
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

output "warehouse_id" {
  description = "Id to give a connection's `warehouse_id`."
  value       = montecarlo_warehouse.snowflake.id
}

output "warehouse_type" {
  description = "The warehouse type. Every connection added has to fit it."
  value       = montecarlo_warehouse.snowflake.type
}
