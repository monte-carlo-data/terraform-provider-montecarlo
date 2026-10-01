# An OAuth client for a generic collection agent: a client id and secret the agent exchanges for
# short-lived access tokens on its own.
#
# The client belongs to the deployment, not to the registered agent, so it can be minted before
# the agent exists. The secret is returned by the create and never again, so this resource holds
# it in state: anyone who can read the state can read the secret. Keep state in a backend that
# encrypts it and limits who can read it. `secret_id` names that initial secret, for when the
# client's secrets can be rotated in place. Until then, rotate by adding a second client, moving
# the agent onto it, and removing the first.
#
# Hand the secret on through a write-only argument, as below, so it is not stored a second time.
# A plain output, even a sensitive one, is stored in state too.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.50"
    }
  }
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # the provider documentation for the alternatives.
}

provider "aws" {
  region = "us-east-1"
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

resource "aws_secretsmanager_secret" "agent_oauth" {
  name = "mcd/agent/oauth"
}

resource "aws_secretsmanager_secret_version" "agent_oauth" {
  secret_id = aws_secretsmanager_secret.agent_oauth.id
  # What the agent reads from its credentials file: `client_id` is the same value as `id`.
  secret_string_wo = jsonencode({
    client_id     = montecarlo_generic_collection_agent_oauth_client.agent.client_id
    client_secret = montecarlo_generic_collection_agent_oauth_client.agent.client_secret
  })
  # Bump when the client is replaced, so the new secret is written.
  secret_string_wo_version = 1
}

output "credentials_secret_arn" {
  description = "Secrets Manager secret holding the agent's OAuth credentials file."
  value       = aws_secretsmanager_secret.agent_oauth.arn
}

output "expiration_time" {
  description = "When the client stops being accepted."
  value       = montecarlo_generic_collection_agent_oauth_client.agent.expiration_time
}
