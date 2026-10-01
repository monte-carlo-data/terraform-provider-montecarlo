# A deployment is what an agent or a data store registers onto. It comes first in every flow.
#
# On AWS, creating it generates an external id that the assumable role has to trust, which is
# why the deployment cannot be created alongside the AWS resources — they need its output. On
# Azure there is no external id, so only the registration depends on the deployment.

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

output "external_id" {
  description = "External id to trust in the agent's assumable role. Empty on platforms other than AWS."
  value       = montecarlo_deployment.agent.aws_external_id
}

output "deployment_enabled" {
  description = "Whether the deployment can serve connections, which it can once an agent is registered."
  value       = montecarlo_deployment.agent.enabled
}
