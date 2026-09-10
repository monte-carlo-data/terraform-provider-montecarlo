# A token for a generic collection agent: a key id and secret the agent sends as headers on
# every request.
#
# The token belongs to the deployment, not to the registered agent, so it can be minted before
# the agent exists. The secret is returned by the create and never again; Terraform holds it in
# state like any generated credential. A deployment can hold any number of tokens, so to rotate,
# add a second one, move the agent onto it, and remove the first.

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
  runtime_platform = "GENERIC"
}

resource "montecarlo_generic_collection_agent_token" "agent" {
  deployment_id = montecarlo_deployment.agent.id
  description   = "production agent"
}

# What the agent reads from its token file: `mcd_id` is the same value as `id`.
output "token_file" {
  description = "Contents of the agent's token file."
  sensitive   = true
  value = jsonencode({
    mcd_id    = montecarlo_generic_collection_agent_token.agent.mcd_id
    mcd_token = montecarlo_generic_collection_agent_token.agent.mcd_token
  })
}
