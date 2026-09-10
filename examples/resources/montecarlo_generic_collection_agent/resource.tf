# A generic collection agent, end to end: the deployment, the credential the agent presents,
# the agent itself on EKS, and the registration that enables it.
#
#   https://docs.getmontecarlo.com/docs/generic-agent-platforms
#
# Registration succeeds once the agent is running with the deployment's credential. Before that
# it answers 503, which the provider retries for a bounded time before failing the apply; apply
# again once the agent is up.
#
# The EKS module below is one way to run the agent. The AKS and GKE modules take the same
# `backend_service_url` and `oauth_credentials` inputs:
#
#   https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/azurerm
#   https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/google
#
# Or run the agent yourself, on Docker Compose or a cluster you already have, with the same
# client id and secret; the docs page above covers those.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # ../../provider/provider.tf for the alternatives.
}

# The EKS module does not configure the AWS provider; the root module does.
provider "aws" {
  region = "us-east-1"
}

variable "backend_service_url" {
  description = "The agent service endpoint, from Monte Carlo -> Account information -> Agent Service -> Public endpoint."
  type        = string
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

# https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/aws
# A new EKS cluster with the agent installed by Helm. The module stores the OAuth client in
# Secrets Manager and points the chart at it.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-k8s-agent/aws"
  version = "~> 0.1"

  backend_service_url = var.backend_service_url

  oauth_credentials = {
    client_id     = montecarlo_generic_collection_agent_oauth_client.agent.client_id
    client_secret = montecarlo_generic_collection_agent_oauth_client.agent.client_secret
  }

  helm = {
    # https://hub.docker.com/r/montecarlodata/generic-agent-helm/tags
    chart_version = "0.1.12"
  }
}

# The module is done once Helm has installed the release; the pods can take a little longer to
# connect, which is the window the provider's retries cover.
resource "montecarlo_generic_collection_agent" "agent" {
  deployment_id = montecarlo_deployment.agent.id

  depends_on = [module.mcd_agent]
}

output "cluster_name" {
  description = "The EKS cluster running the agent."
  value       = module.mcd_agent.cluster_name
}

output "agent_enabled" {
  description = "Whether Monte Carlo reached the agent and is using it."
  value       = montecarlo_generic_collection_agent.agent.enabled
}
