# An AWS collection agent, end to end: the deployment, the agent's infrastructure, and the
# registration that ties them together.
#
# The ordering is not cosmetic. Creating the deployment generates an external id; the agent's
# assumable role has to trust that id, so the deployment comes first, the module takes the id as
# an input, and the registration reads the module's outputs back. Without the provider this is
# the manual step of copying an external id out of the Monte Carlo UI.

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
  # ../../provider/provider.tf for the alternatives.
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

output "deployment_id" {
  description = "Id of the deployment the agent is registered on."
  value       = montecarlo_deployment.agent.id
}

output "agent_enabled" {
  description = "Whether the deployment can serve connections, which it can once the agent is registered."
  value       = montecarlo_deployment.agent.enabled
}
