# An Azure collection agent, end to end: the deployment, the agent's Function
# App, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/create-and-register-an-azure-agent
#
# The deployment and the module are independent, and only the registration
# depends on both. Nothing has to exist before the Azure resources are built:
# the credential Monte Carlo uses comes out of them. That is the difference from
# AWS, where the deployment's external id has to be created first so the role
# can trust it.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    # The agent module caps azurerm below 4.0.
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 3.75, < 4.0.0"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AZURE"
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/azurerm
#
# `auth_type` decides how Monte Carlo authenticates to the Function App. The
# default, AZURE_FUNCTION_APP_KEY, uses the app's host key and needs no
# directory permissions. The alternative, AZURE_FUNCTION_SERVICE_PRINCIPAL, has
# the module create an Entra ID app registration and a caller principal, which
# requires the Cloud Application Administrator role in your tenant. Moving back
# off it later means destroying the agent first: choosing it turns on the
# Function App's built-in authentication, and Azure rejects any later attempt to
# turn that back off, so the apply fails partway instead of switching.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/azurerm"
  version = "~> 1.0"

  location  = "EAST US"
  auth_type = "AZURE_FUNCTION_APP_KEY"
}

# The module does not output the host key, so it is read back off the Function
# App it built. `default_function_key` is the key Monte Carlo sends as
# `x-functions-key`; `primary_key` is the master key, which grants more than the
# agent needs.
data "azurerm_function_app_host_keys" "agent" {
  name                = module.mcd_agent.mcd_agent_function_name
  resource_group_name = module.mcd_agent.mcd_agent_resource_group_name
}

# `authentication_type` is the discriminator, and exactly one matching
# credential block is accepted. Reading it from the module means the two cannot
# disagree.
#
# The key is write-only here, but the data source above still holds it in state.
# A rotated key alone plans nothing: bump the version with it.
#
# Under AZURE_FUNCTION_SERVICE_PRINCIPAL, replace the `function_app_key` block
# with:
#
#   service_principal = {
#     audience                 = module.mcd_agent.mcd_agent_sp_audience
#     client_id                = module.mcd_agent.mcd_agent_sp_client_id
#     client_secret_wo         = module.mcd_agent.mcd_agent_sp_client_secret
#     client_secret_wo_version = 1
#     tenant_id                = module.mcd_agent.mcd_agent_sp_tenant_id
#   }
resource "montecarlo_azure_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  authentication_type = module.mcd_agent.mcd_agent_auth_type
  function_app_url    = module.mcd_agent.mcd_agent_function_url

  function_app_key = {
    app_key_wo         = data.azurerm_function_app_host_keys.agent.default_function_key
    app_key_wo_version = 1
  }
}

output "function_app_url" {
  description = "URL Monte Carlo calls to reach the agent."
  value       = module.mcd_agent.mcd_agent_function_url
}

output "agent_enabled" {
  description = "Whether Monte Carlo validated the agent and is using it."
  value       = montecarlo_azure_collection_agent.agent.enabled
}
