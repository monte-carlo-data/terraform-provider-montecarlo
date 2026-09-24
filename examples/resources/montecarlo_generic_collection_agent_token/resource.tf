# A token for a generic collection agent: a key id and secret the agent sends as headers on
# every request.
#
# The token belongs to the deployment, not to the registered agent, so it can be minted before
# the agent exists. The secret is returned by the create and never again, so this resource holds
# it in state: anyone who can read the state can read the secret. Keep state in a backend that
# encrypts it and limits who can read it. A deployment can hold any number of tokens, so to
# rotate, add a second one, move the agent onto it, and remove the first.
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
  # ../../provider/provider.tf for the alternatives.
}

provider "aws" {
  region = "us-east-1"
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
resource "aws_secretsmanager_secret" "agent_token" {
  name = "mcd/agent/token"
}

resource "aws_secretsmanager_secret_version" "agent_token" {
  secret_id = aws_secretsmanager_secret.agent_token.id
  secret_string_wo = jsonencode({
    mcd_id    = montecarlo_generic_collection_agent_token.agent.mcd_id
    mcd_token = montecarlo_generic_collection_agent_token.agent.mcd_token
  })
  # Bump when the token is replaced, so the new secret is written.
  secret_string_wo_version = 1
}

output "token_secret_arn" {
  description = "Secrets Manager secret holding the agent's token file."
  value       = aws_secretsmanager_secret.agent_token.arn
}
