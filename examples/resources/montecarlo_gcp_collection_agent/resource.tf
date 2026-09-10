# A GCP collection agent, end to end: the deployment, the agent's Cloud Run service, and the
# registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/create-and-register-a-gcp-agent
#
# The deployment and the module are independent, and only the registration depends on both.
# The credential Monte Carlo authenticates with, a service account key, comes out of the module,
# so nothing has to exist before the GCP resources are built.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 6.0"
    }
  }
}

provider "google" {
  project = "my-project"
  region  = "us-east4"
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # ../../provider/provider.tf for the alternatives.
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "GCP"
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/google
# `generate_key` mints the invoker key Monte Carlo authenticates with.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/google"
  version = "~> 1.0"

  project_id   = "my-project"
  location     = "us-east4"
  generate_key = true
}

# The module returns the key base64-encoded, as the google provider does; the API takes the
# key file's contents. Under CUSTOM_AUTH_HEADERS, replace `service_account_key` with:
#
#   auth_headers = {
#     headers = { "x-api-key" = var.agent_api_key }
#   }
resource "montecarlo_gcp_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  authentication_type = "GCP_JSON_SERVICE_ACCOUNT_KEY"
  cloud_run_url       = module.mcd_agent.mcd_agent_uri
  service_account_key = base64decode(module.mcd_agent.mcd_agent_invoker_key[0])
}

output "cloud_run_url" {
  description = "URL Monte Carlo calls to reach the agent."
  value       = module.mcd_agent.mcd_agent_uri
}

output "agent_enabled" {
  description = "Whether Monte Carlo validated the agent and is using it."
  value       = montecarlo_gcp_collection_agent.agent.enabled
}
