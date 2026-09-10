# An OAuth client for a generic collection agent: a client id and secret the agent exchanges for
# short-lived access tokens on its own.
#
# The client belongs to the deployment, not to the registered agent, so it can be minted before
# the agent exists. The secret is returned by the create and never again; Terraform holds it in
# state like any generated credential. `secret_id` names that initial secret, for when the
# client's secrets can be rotated in place. Until then, rotate by adding a second client, moving
# the agent onto it, and removing the first.

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

resource "montecarlo_generic_collection_agent_oauth_client" "agent" {
  deployment_id = montecarlo_deployment.agent.id
  description   = "production agent"

  # Leave it out for a client that does not expire.
  expiration_days = 365
}

# What the agent reads from its credentials file: `client_id` is the same value as `id`.
output "credentials_file" {
  description = "Contents of the agent's OAuth credentials file."
  sensitive   = true
  value = jsonencode({
    client_id     = montecarlo_generic_collection_agent_oauth_client.agent.client_id
    client_secret = montecarlo_generic_collection_agent_oauth_client.agent.client_secret
  })
}

output "expiration_time" {
  description = "When the client stops being accepted."
  value       = montecarlo_generic_collection_agent_oauth_client.agent.expiration_time
}
