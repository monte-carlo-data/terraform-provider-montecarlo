# A generic collection agent: the deployment, the credential the agent presents, and the
# registration that enables it.
#
#   https://docs.getmontecarlo.com/docs/generic-agent-platforms
#
# A generic agent connects out to Monte Carlo, so there is no infrastructure to register: the
# agent runs wherever you start it, with a credential minted for its deployment. Registration
# checks that the agent has connected and enables it, so it can only succeed once the agent is
# running with that credential. Registering earlier answers 503, which the provider retries for a
# bounded time before failing the apply; apply again once the agent is up.

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

# The credential. An OAuth client here; see ../montecarlo_generic_collection_agent_token for
# the other kind. The secret is returned once and held in state.
resource "montecarlo_generic_collection_agent_oauth_client" "agent" {
  deployment_id = montecarlo_deployment.agent.id
}

# Start the agent with the client id and secret (for the Docker Compose example, the contents of
# secrets/oauth.json), then register. Nothing in the configuration forces that order: the
# dependency is on the agent running, which Terraform cannot see. `depends_on` only keeps the
# destroy ordered, agent first.
resource "montecarlo_generic_collection_agent" "agent" {
  deployment_id = montecarlo_deployment.agent.id

  depends_on = [montecarlo_generic_collection_agent_oauth_client.agent]
}

output "agent_credentials" {
  description = "The agent's credentials file, as the agent reads it."
  sensitive   = true
  value = jsonencode({
    client_id     = montecarlo_generic_collection_agent_oauth_client.agent.client_id
    client_secret = montecarlo_generic_collection_agent_oauth_client.agent.client_secret
  })
}

output "agent_enabled" {
  description = "Whether Monte Carlo reached the agent and is using it."
  value       = montecarlo_generic_collection_agent.agent.enabled
}
