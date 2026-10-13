---
page_title: "Deploy a collection agent"
description: |-
  Run Monte Carlo's collection agent, or a data store, in your own cloud, and register it with Monte Carlo.
---

# Deploy a collection agent

A deployment is where Monte Carlo's queries against your data run. Monte Carlo hosts one for your account. To keep that traffic in your own cloud instead, create a deployment and register either of these on it:

- A **collection agent**: a service in your cloud that runs Monte Carlo's queries and sends back the results. It exists on AWS, Azure and GCP, and as a generic agent you run on Kubernetes, Docker Compose or anything else.
- A **data store**: a bucket or container in your cloud. Monte Carlo connects to your warehouse directly and keeps what it collects there. It exists on AWS, Azure and GCP.

A deployment holds one or the other, and its `type` (`COLLECTION_AGENT` or `COLLECTION_DATA_STORE`) says which. A warehouse, a BI container or an ETL container then goes on the deployment through its `deployment_id`; see [Connect a Snowflake warehouse](connect-a-snowflake-warehouse).

Every flow has the same three parts:

1. `montecarlo_deployment`, which comes first.
2. The infrastructure in your cloud. For an agent, Monte Carlo publishes a module per platform. A data store has no module, so its bucket or container is defined directly.
3. The registration (`montecarlo_<platform>_collection_agent` or `montecarlo_<platform>_collection_data_store`), which hands Monte Carlo the endpoint and the credential it reaches your infrastructure with.

## AWS

Creating an AWS deployment generates `aws_external_id`. The role Monte Carlo assumes has to trust that id, so the deployment comes before the agent's infrastructure. The [`mcd-agent` module](https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws) takes the id as an input, and the registration reads the module's outputs back.

```terraform
# An AWS collection agent, end to end: the deployment, the agent's
# infrastructure, and the registration that ties them together.
#
# The ordering is not cosmetic. Creating the deployment generates an external
# id; the agent's assumable role has to trust that id, so the deployment comes
# first, the module takes the id as an input, and the registration reads the
# module's outputs back. Without the provider this is the manual step of copying
# an external id out of the Monte Carlo UI.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AWS"
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws
# The module configures the AWS provider itself, from its own `region` variable.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/aws"
  version = "~> 1.0"

  region = "us-east-1"

  # The external id the deployment generated. This is the dependency that forces
  # the ordering.
  external_id = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}

output "deployment_id" {
  description = "Id of the deployment the agent is registered on."
  value       = montecarlo_deployment.agent.id
}

output "agent_enabled" {
  description = "Whether the deployment can serve connections, which it can once the agent is registered."
  value       = montecarlo_deployment.agent.enabled
}
```

An AWS data store follows the same order, for the same reason:

```terraform
# An AWS data store, end to end: the deployment, the bucket and role Monte Carlo
# reaches, and the registration that ties them together.
#
# Same ordering as the collection agent, and for the same reason. Creating the
# deployment generates an external id; the role Monte Carlo assumes has to trust
# it, so the deployment comes first and the registration reads the bucket and
# role back.
#
#   https://docs.getmontecarlo.com/docs/direct-connection-with-an-aws-data-store
#
# There is no published module for a data store, so the bucket and the role are
# defined here.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "us-east-1"
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

# The Monte Carlo account that assumes the role, which differs per Monte Carlo
# deployment. The value below is one deployment's account and may not be yours.
# Take yours from the Account Information page in the product, under Collection,
# as "AWS account ID", and substitute it before applying.
locals {
  mcd_account_id = "590183797493"
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "AWS"
}

# A prefix rather than a fixed name: bucket names are globally unique, so a
# fixed one collides with any other copy of this example.
resource "aws_s3_bucket" "store" {
  bucket_prefix = "mcd-data-store-"
}

resource "aws_s3_bucket_public_access_block" "store" {
  bucket = aws_s3_bucket.store.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "store" {
  bucket = aws_s3_bucket.store.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Monte Carlo writes query output, root-cause samples and idempotency markers
# under their own prefixes. None of it is worth keeping longer than 90 days.
resource "aws_s3_bucket_lifecycle_configuration" "store" {
  bucket = aws_s3_bucket.store.id

  rule {
    id     = "custom-sql-output-samples-expiration"
    status = "Enabled"

    filter {
      prefix = "custom-sql-output-samples/"
    }

    expiration {
      days = 90
    }
  }

  rule {
    id     = "rca-expiration"
    status = "Enabled"

    filter {
      prefix = "rca"
    }

    expiration {
      days = 90
    }
  }

  rule {
    id     = "idempotent-expiration"
    status = "Enabled"

    filter {
      prefix = "idempotent/"
    }

    expiration {
      days = 90
    }
  }
}

# The role Monte Carlo assumes, trusting the external id the deployment
# generated.
resource "aws_iam_role" "store" {
  name_prefix = "mcd-data-store-"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { AWS = "arn:aws:iam::${local.mcd_account_id}:root" }
      Condition = {
        StringEquals = {
          "sts:ExternalId" = montecarlo_deployment.data_store.aws_external_id
        }
      }
    }]
  })
}

# The bucket-level reads matter as much as the object ones. Monte Carlo
# validates storage access when the store is registered, and a policy missing
# them registers and then fails that validation.
resource "aws_iam_role_policy" "store" {
  name = "s3-policy"
  role = aws_iam_role.store.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketPublicAccessBlock",
        "s3:GetBucketPolicyStatus",
        "s3:GetBucketAcl",
      ]
      Resource = [
        aws_s3_bucket.store.arn,
        "${aws_s3_bucket.store.arn}/*",
      ]
    }]
  })
}

# Registering validates storage access, so it has to wait for the policy and the
# bucket's own settings. The reference to the role's ARN alone would let it run
# as soon as the role existed.
resource "montecarlo_aws_collection_data_store" "store" {
  deployment_id = montecarlo_deployment.data_store.id
  bucket_name   = aws_s3_bucket.store.bucket
  role_arn      = aws_iam_role.store.arn

  depends_on = [
    aws_iam_role_policy.store,
    aws_s3_bucket_public_access_block.store,
    aws_s3_bucket_lifecycle_configuration.store,
    aws_s3_bucket_server_side_encryption_configuration.store,
  ]
}

output "bucket_name" {
  description = "Bucket Monte Carlo reads and writes."
  value       = aws_s3_bucket.store.bucket
}

output "data_store_enabled" {
  description = "Whether Monte Carlo validated the store and is using it."
  value       = montecarlo_aws_collection_data_store.store.enabled
}
```

## Azure

Azure has no external id. The deployment and the Function App are independent, and only the registration depends on both. The registration authenticates with either the Function App's key or a service principal. Pick one up front: the service principal path adds authentication settings to the Function App that the azurerm provider can't remove, so moving off it means destroying and re-creating the agent.

```terraform
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
```

An Azure blob data store:

```terraform
# An Azure blob data store, end to end: the deployment, the storage account and
# container Monte Carlo reaches, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/create-and-register-an-azure-blob-data-store
#
# The deployment and the storage account are independent, and only the
# registration depends on both. Nothing has to exist before the Azure resources
# are built: the credential Monte Carlo reaches the container with comes out of
# them.

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
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "AZURE"
}

# Storage account names are globally unique and allow no punctuation, so the
# suffix is random. The resource group takes the same suffix, which keeps
# everything one apply creates together.
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

# Monte Carlo writes query output, root-cause samples and idempotency markers
# under their own prefixes. None of it is worth keeping longer than 90 days.
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

# `authentication_type` is the discriminator, and exactly one matching
# credential block is accepted. The connection string is the whole account's
# key. It is write-only here, but azurerm_storage_account still holds it in
# state, so use a backend that encrypts state. A rotated key alone plans
# nothing: bump the version with it.
#
# The alternative is AZURE_STORAGE_SERVICE_PRINCIPAL, which authenticates as an
# Entra ID application granted `Storage Blob Data Contributor` on the account
# rather than sharing the account key. It replaces the `storage_account_keys`
# block with:
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
# Registering validates storage access, so it has to wait for the lifecycle
# policy too — the container reference alone would let it run as soon as the
# container existed.
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
```

## GCP

GCP has no external id either. The registration authenticates with a service account key that the module creates.

```terraform
# A GCP collection agent, end to end: the deployment, the agent's Cloud Run
# service, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/create-and-register-a-gcp-agent
#
# The deployment and the module are independent, and only the registration
# depends on both. The credential Monte Carlo authenticates with, a service
# account key, comes out of the module, so nothing has to exist before the GCP
# resources are built.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

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
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
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

# The module returns the key base64-encoded, as the google provider does; the
# API takes the key file's contents. The key is write-only here, but the module
# still holds it in state. A new key alone plans nothing: bump the version with
# it.
#
# Under CUSTOM_AUTH_HEADERS, replace `service_account_key_wo` and
# `service_account_key_wo_version` with:
#
#   auth_headers = {
#     headers_wo         = { "x-api-key" = var.agent_api_key }
#     headers_wo_version = 1
#   }
resource "montecarlo_gcp_collection_agent" "agent" {
  deployment_id                  = montecarlo_deployment.agent.id
  authentication_type            = "GCP_JSON_SERVICE_ACCOUNT_KEY"
  cloud_run_url                  = module.mcd_agent.mcd_agent_uri
  service_account_key_wo         = base64decode(module.mcd_agent.mcd_agent_invoker_key[0])
  service_account_key_wo_version = 1
}

output "cloud_run_url" {
  description = "URL Monte Carlo calls to reach the agent."
  value       = module.mcd_agent.mcd_agent_uri
}

output "agent_enabled" {
  description = "Whether Monte Carlo validated the agent and is using it."
  value       = montecarlo_gcp_collection_agent.agent.enabled
}
```

A GCP data store:

```terraform
# A GCP data store, end to end: the deployment, the bucket and service account
# Monte Carlo reaches, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/deployment-and-connecting
#
# There is no published module for a data store, so the bucket, role, service
# account and key are defined here, following Monte Carlo's template in
# mcd-public-resources (templates/terraform/gcp_data_store). No external id on
# GCP, so only the registration depends on the deployment.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

provider "google" {
  project = "my-project"
  region  = "us-east4"
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "GCP"
}

# Suffixed rather than fixed: bucket names are globally unique, so a fixed one
# collides with any other copy of this example.
resource "random_id" "store" {
  byte_length = 4
}

resource "google_storage_bucket" "store" {
  name     = "mcd-store-${random_id.store.hex}"
  location = "us-east4"

  lifecycle_rule {
    condition {
      age            = 90
      matches_prefix = ["custom-sql-output-samples/", "rca", "idempotent"]
    }
    action {
      type = "Delete"
    }
  }

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
}

# The bucket-level reads matter as much as the object ones: the collector's
# storage validation exercises them, so a role missing them registers and then
# fails validation.
resource "google_project_iam_custom_role" "store" {
  role_id = "mcdStoreRole${random_id.store.hex}"
  title   = "MCD Store Role"
  permissions = [
    "storage.objects.create",
    "storage.objects.delete",
    "storage.objects.get",
    "storage.objects.list",
    "storage.objects.update",
    "storage.buckets.get",
    "storage.buckets.getIamPolicy",
  ]
}

resource "google_service_account" "store" {
  account_id   = "mcd-store-sa-${random_id.store.hex}"
  display_name = "Monte Carlo data store"
}

resource "google_storage_bucket_iam_binding" "store" {
  bucket  = google_storage_bucket.store.name
  role    = google_project_iam_custom_role.store.id
  members = ["serviceAccount:${google_service_account.store.email}"]
}

resource "google_service_account_key" "store" {
  service_account_id = google_service_account.store.name
}

# The key arrives base64-encoded from the google provider; the API takes the key
# file's contents. The registration waits for the IAM binding, not only the key:
# the collector validates storage access during registration.
#
# The key is write-only here, but google_service_account_key still holds it in
# state. Replacing that key alone plans nothing here: bump the version with it.
resource "montecarlo_gcp_collection_data_store" "store" {
  deployment_id                  = montecarlo_deployment.data_store.id
  bucket_name                    = google_storage_bucket.store.name
  service_account_key_wo         = base64decode(google_service_account_key.store.private_key)
  service_account_key_wo_version = 1

  depends_on = [google_storage_bucket_iam_binding.store]
}

output "bucket_name" {
  description = "Bucket Monte Carlo writes query results and samples to."
  value       = google_storage_bucket.store.name
}

output "data_store_enabled" {
  description = "Whether Monte Carlo validated the store and is using it."
  value       = montecarlo_gcp_collection_data_store.store.enabled
}
```

## Generic agent

A generic agent runs wherever you run it, so it authenticates to Monte Carlo rather than the other way round. Mint a credential on the deployment and hand it to the agent:

- `montecarlo_generic_collection_agent_oauth_client`: a client id and secret the agent exchanges for short-lived access tokens.
- `montecarlo_generic_collection_agent_token`: a key id and secret the agent sends on every request.

The credential belongs to the deployment, so it can be minted before the agent exists. Its secret is returned once and kept in state, so keep state in a backend that encrypts it and limits who can read it. Pass the secret on through a write-only argument, as below, so it isn't stored a second time.

Registration succeeds once the agent is running with that credential. Until then the agent answers 503. The provider retries for a while, then fails the apply; apply again once the agent is up. This example runs the agent on EKS with the [`mcd-k8s-agent` module](https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/aws). The AKS and GKE modules take the same inputs.

```terraform
# A generic collection agent, end to end: the deployment, the credential the
# agent presents, the agent itself on EKS, and the registration that enables it.
#
#   https://docs.getmontecarlo.com/docs/generic-agent-platforms
#
# Registration succeeds once the agent is running with the deployment's
# credential. Before that it answers 503, which the provider retries for a
# bounded time before failing the apply; apply again once the agent is up.
#
# The EKS module below is one way to run the agent. The AKS and GKE modules take
# the same `backend_service_url` and `oauth_secret` inputs, with the secret in
# Key Vault or Secret Manager instead of Secrets Manager:
#
#   https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/azurerm
#   https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/google
#
# Or run the agent yourself, on Docker Compose or a cluster you already have,
# with the same client id and secret; the docs page above covers those.

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
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
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

# The credential. An OAuth client here; see the
# `montecarlo_generic_collection_agent_token` example for the other kind. The
# secret is returned once, so this resource holds it in state: anyone who can
# read the state can read it. Keep state in a backend that encrypts it and
# limits access.
resource "montecarlo_generic_collection_agent_oauth_client" "agent" {
  deployment_id = montecarlo_deployment.agent.id
}

# The secret goes to Secrets Manager through a write-only argument, so this is
# the only other copy and it is not in state. Handing it to the module's
# `oauth_credentials` instead would store it a second time. The JSON keys are
# the ones the module's chart reads.
resource "aws_secretsmanager_secret" "mcd_agent_oauth" {
  name = "mcd/agent/${montecarlo_deployment.agent.name}/oauth"
}

resource "aws_secretsmanager_secret_version" "mcd_agent_oauth" {
  secret_id = aws_secretsmanager_secret.mcd_agent_oauth.id
  secret_string_wo = jsonencode({
    client_id     = montecarlo_generic_collection_agent_oauth_client.agent.client_id
    client_secret = montecarlo_generic_collection_agent_oauth_client.agent.client_secret
  })
  # Bump when the client is replaced, so the new secret is written.
  secret_string_wo_version = 1
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-k8s-agent/aws
# A new EKS cluster with the agent installed by Helm, pointed at the secret
# above.
#
# With `create = false` the module grants the agent read access to every secret
# whose name starts with `name`, rather than to this one secret's ARN, so the
# example names it per deployment: no other deployment's secret shares it as a
# prefix.
module "mcd_agent" {
  source = "monte-carlo-data/mcd-k8s-agent/aws"
  # 0.1.4 is the first to take an existing secret.
  version = "~> 0.1, >= 0.1.4"

  backend_service_url = var.backend_service_url

  oauth_secret = {
    create = false
    name   = aws_secretsmanager_secret.mcd_agent_oauth.name
  }
  # Required alongside `oauth_secret`: otherwise the module asks for token
  # credentials. The agent authenticates with the OAuth client, so no token
  # secret is created or read.
  token_secret = {
    create = false
  }

  helm = {
    # https://hub.docker.com/r/montecarlodata/generic-agent-helm/tags
    chart_version = "0.1.12"
  }
}

# The module is done once Helm has installed the release; the pods can take a
# little longer to connect, which is the window the provider's retries cover.
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
```

The [generic agent platforms](https://docs.getmontecarlo.com/docs/generic-agent-platforms) page in the Monte Carlo docs covers Docker Compose and clusters you already run.
