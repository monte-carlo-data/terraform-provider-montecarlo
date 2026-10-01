# An Azure blob data store, end to end: the deployment, the storage account and container
# Monte Carlo reaches, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/create-and-register-an-azure-blob-data-store
#
# The deployment and the storage account are independent, and only the registration depends on
# both. Nothing has to exist before the Azure resources are built: the credential Monte Carlo
# reaches the container with comes out of them.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # the provider documentation for the alternatives.
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "AZURE"
}

# Storage account names are globally unique and allow no punctuation, so the suffix is random.
# The resource group takes the same suffix, which keeps everything one apply creates together.
resource "random_id" "store" {
  byte_length = 4
}

resource "azurerm_resource_group" "store" {
  name     = "mcd-data-store-group-${random_id.store.hex}"
  location = "EAST US"
}

resource "azurerm_storage_account" "store" {
  name                = "mcdstore${random_id.store.hex}"
  resource_group_name = azurerm_resource_group.store.name
  location            = azurerm_resource_group.store.location

  account_tier             = "Standard"
  account_replication_type = "LRS"

  https_traffic_only_enabled        = true
  allow_nested_items_to_be_public   = false
  min_tls_version                   = "TLS1_2"
  cross_tenant_replication_enabled  = false
  infrastructure_encryption_enabled = true
}

# Monte Carlo writes query output, root-cause samples and idempotency markers under their own
# prefixes. None of it is worth keeping longer than 90 days.
resource "azurerm_storage_management_policy" "store" {
  storage_account_id = azurerm_storage_account.store.id

  rule {
    name    = "custom-sql-output-samples-expiration"
    enabled = true

    filters {
      prefix_match = ["custom-sql-output-samples/"]
      blob_types   = ["blockBlob"]
    }

    actions {
      base_blob {
        delete_after_days_since_modification_greater_than = 90
      }
    }
  }

  rule {
    name    = "rca-expiration"
    enabled = true

    filters {
      prefix_match = ["rca"]
      blob_types   = ["blockBlob"]
    }

    actions {
      base_blob {
        delete_after_days_since_modification_greater_than = 90
      }
    }
  }

  rule {
    name    = "idempotent-expiration"
    enabled = true

    filters {
      prefix_match = ["idempotent"]
      blob_types   = ["blockBlob"]
    }

    actions {
      base_blob {
        delete_after_days_since_modification_greater_than = 90
      }
    }
  }
}

resource "azurerm_storage_container" "store" {
  name                  = "mcd-store"
  storage_account_id    = azurerm_storage_account.store.id
  container_access_type = "private"
}

# `authentication_type` is the discriminator, and exactly one matching credential block is
# accepted. The connection string is the whole account's key. It is write-only here, but
# azurerm_storage_account still holds it in state, so use a backend that encrypts state.
# A rotated key alone plans nothing: bump the version with it.
#
# The alternative is AZURE_STORAGE_SERVICE_PRINCIPAL, which authenticates as an Entra ID
# application granted `Storage Blob Data Contributor` on the account rather than sharing the
# account key. It replaces the `storage_account_keys` block with:
#
#   service_principal = {
#     account_name             = azurerm_storage_account.store.name
#     account_url              = trimsuffix(azurerm_storage_account.store.primary_blob_endpoint, "/")
#     client_id                = <the application's client id>
#     client_secret_wo         = <the application's client secret>
#     client_secret_wo_version = 1
#     tenant_id                = <your tenant id>
#   }
#
# Registering validates storage access, so it has to wait for the lifecycle policy too — the
# container reference alone would let it run as soon as the container existed.
resource "montecarlo_azure_collection_data_store" "store" {
  deployment_id       = montecarlo_deployment.data_store.id
  authentication_type = "AZURE_STORAGE_ACCOUNT_KEYS"
  container_name      = azurerm_storage_container.store.name

  storage_account_keys = {
    connection_string_wo         = azurerm_storage_account.store.primary_connection_string
    connection_string_wo_version = 1
  }

  depends_on = [azurerm_storage_management_policy.store]
}

output "storage_account_name" {
  description = "Storage account holding the container Monte Carlo reads and writes."
  value       = azurerm_storage_account.store.name
}

output "data_store_enabled" {
  description = "Whether Monte Carlo validated the store and is using it."
  value       = montecarlo_azure_collection_data_store.store.enabled
}
