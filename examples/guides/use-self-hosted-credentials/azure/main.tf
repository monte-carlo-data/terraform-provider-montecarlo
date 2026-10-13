# Snowflake credentials in Azure Key Vault, read by an Azure collection agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    azurerm = {
      source = "hashicorp/azurerm"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

variable "agent_principal_id" {
  description = "Principal id of the agent's managed identity. The agent module outputs it as mcd_agent_service_identity_principal_id."
  type        = string
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

data "azurerm_key_vault" "secrets" {
  name                = "my-key-vault"
  resource_group_name = "my-resource-group"
}

# The agent module grants no Key Vault access. The vault must use Azure RBAC.
resource "azurerm_role_assignment" "agent_read_secret" {
  scope                = data.azurerm_key_vault.secrets.id
  role_definition_name = "Key Vault Secrets User"
  principal_id         = var.agent_principal_id
}

resource "montecarlo_self_hosted_azure_credentials" "snowflake" {
  connection_type = "snowflake"
  akv_secret      = "monte-carlo-snowflake"
  akv_vault_name  = data.azurerm_key_vault.secrets.name

  # The grant has to exist before Monte Carlo is told where the secret is.
  depends_on = [azurerm_role_assignment.agent_read_secret]
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_azure_credentials.snowflake.id
}
