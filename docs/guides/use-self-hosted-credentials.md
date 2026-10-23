---
page_title: "Use self-hosted credentials"
description: |-
  Keep a connection's secret in your own secret store or on your agent, and have Monte Carlo store only where to find it.
---

# Use self-hosted credentials

Monte Carlo managed credentials, such as `montecarlo_snowflake_credentials`, give Monte Carlo the secret to keep. Self-hosted credentials keep it on your side: Monte Carlo stores only where the secret is, and your collection agent reads it each time a connection runs.

| Resource | Where the secret is |
|---|---|
| `montecarlo_self_hosted_aws_credentials` | AWS Secrets Manager |
| `montecarlo_self_hosted_azure_credentials` | Azure Key Vault |
| `montecarlo_self_hosted_gcp_credentials` | GCP Secret Manager |
| `montecarlo_self_hosted_env_var_credentials` | An environment variable on the agent, named `MCD_…` |
| `montecarlo_self_hosted_file_credentials` | A file on the agent |

A connection uses them the same way it uses managed credentials, through `credentials_id`. Switching a connection between the two changes nothing else in its configuration, and Terraform updates the connection in place.

## Before you start

Before you use any of these, you need:

- A collection agent that you run, registered on the deployment the warehouse uses. The deployment Monte Carlo hosts has no agent of yours to read the secret. See [Deploy a collection agent](deploy-a-collection-agent).
- The provider configured with an API token or OAuth client credentials. See [Authenticate the provider](authenticate-the-provider).

Two more things have to be true, and neither can be expressed in the Monte Carlo provider:

1. **The secret holds the JSON that the connection type expects.** Each connection type documents its own shape in [Self-hosted credentials](https://docs.getmontecarlo.com/docs/self-hosted-credentials). For Snowflake, `private_key` is the base64 body of the PKCS#8 key, without the BEGIN and END lines that `montecarlo_snowflake_credentials` takes.
2. **The agent can read the secret.** Grant the agent's identity read access to that one secret, and make the credentials resource `depends_on` the grant. Without it, the connection is created and then fails to reach the warehouse.

`connection_type` is fixed when the credentials are created, and it decides the type of any connection that reads with them. For BigQuery, also set `bq_project_id`. For the Databricks types, `sql_warehouse_id` is required.

## AWS Secrets Manager

The agent module gives the agent's execution role no Secrets Manager access, so this example adds a policy scoped to the one secret.

```terraform
# Credentials kept in your own AWS Secrets Manager, which the collection agent
# reads at runtime. Monte Carlo stores only where to find them.
#
# Credentials do nothing until a connection reads with them — see the
# `montecarlo_connection` example for the whole flow. For the alternative, where
# Monte Carlo holds the secret, see the `montecarlo_snowflake_credentials`
# example.
#
# Two things have to be true before this works, and neither is expressible here:
#
# 1. The secret's value is the JSON schema documented for the connection type.
#    For Snowflake that is the object below, and note `private_key` is the
#    PKCS#8 base64 body — the `MII...` text, without the BEGIN and END lines the
#    Monte Carlo managed resource takes:
#
#      {"connect_args": {"user": "MONTE_CARLO", "private_key": "MII...",
#                        "account": "xy12345.us-east-1", "warehouse": "MONTE_CARLO_WH"}}
#
#    Every supported connection type's schema is at
#    https://docs.getmontecarlo.com/docs/self-hosted-credentials
#
# 2. The agent can read the secret. The agent module grants its execution role
#    no Secrets Manager access, so the policy below adds it, scoped to this one
#    secret. Without it the connection is created and then fails to reach
#    Snowflake.

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
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
}

provider "aws" {
  region = "us-east-1"
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AWS"
}

# The Monte Carlo AWS account that invokes the agent. It differs per Monte Carlo
# instance, so read it rather than relying on the module's default.
data "montecarlo_hosting_private_link" "this" {}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/aws"
  version = "~> 1.0"

  region           = "us-east-1"
  cloud_account_id = data.montecarlo_hosting_private_link.this.aws.account_id
  external_id      = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}

# The secret itself is assumed to exist already, holding the JSON described
# above. Replace the ARN with yours.
locals {
  snowflake_secret_arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:monte-carlo/snowflake-AbCdEf"
}

# Point 2 above: the agent reads the secret as itself, so its execution role
# needs the grant.
resource "aws_iam_role_policy" "agent_read_secret" {
  name = "read_snowflake_secret"
  role = module.mcd_agent.mcd_agent_execution_role.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "secretsmanager:GetSecretValue"
      Resource = local.snowflake_secret_arn
    }]
  })
}

resource "montecarlo_self_hosted_aws_credentials" "snowflake" {
  # Fixes what these credentials are for, and so the type of any connection
  # using them.
  connection_type = "snowflake"

  # Name or ARN of the secret.
  aws_secret = local.snowflake_secret_arn

  # Region of the secret. Omit it to use the deployment's own region.
  # aws_region = "us-east-1"

  # Set these only when the agent cannot read the secret as itself — for a
  # secret in another account, say. The role named here has to carry the tag
  # MonteCarloData = "", because that is the only condition under which the
  # agent module lets its execution role assume a role.
  # assumable_role = "arn:aws:iam::123456789012:role/monte-carlo-secret-reader"
  # external_id    = "..."

  # The grant has to exist before Monte Carlo is told where the secret is.
  depends_on = [aws_iam_role_policy.agent_read_secret]
}

output "credentials_id" {
  description = "Id to give a connection's `credentials_id`."
  value       = montecarlo_self_hosted_aws_credentials.snowflake.id
}

output "storage_type" {
  description = "Where the secret lives. aws_secrets_manager for these, since you host them."
  value       = montecarlo_self_hosted_aws_credentials.snowflake.storage_type
}
```

To read a secret in another account, set `assumable_role` and `external_id`. The role has to carry the tag `MonteCarloData = ""`: the agent module lets its execution role assume only roles tagged that way.

## Azure Key Vault

The agent module gives the agent's managed identity no Key Vault access, so this example grants it the "Key Vault Secrets User" role on the vault. The vault must use Azure RBAC. The module outputs the identity's principal id as `mcd_agent_service_identity_principal_id`.

Name the vault with `akv_vault_name`, `akv_vault_url`, or both.

```terraform
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
```

## GCP Secret Manager

The agent module gives the agent's service account no Secret Manager access, so this example grants it `roles/secretmanager.secretAccessor` on the one secret. The service account is the one the agent's Cloud Run service runs as.

```terraform
# Snowflake credentials in GCP Secret Manager, read by a GCP collection agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
    google = {
      source = "hashicorp/google"
    }
  }
}

provider "google" {
  project = "my-project"
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

variable "agent_service_account_email" {
  description = "Email of the service account the agent's Cloud Run service runs as. The agent module does not output it."
  type        = string
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

locals {
  secret_name = "monte-carlo-snowflake"
}

# The agent module grants no Secret Manager access.
resource "google_secret_manager_secret_iam_member" "agent_read_secret" {
  secret_id = local.secret_name
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${var.agent_service_account_email}"
}

resource "montecarlo_self_hosted_gcp_credentials" "snowflake" {
  connection_type = "snowflake"
  gcp_secret      = local.secret_name

  # The grant has to exist before Monte Carlo is told where the secret is.
  depends_on = [google_secret_manager_secret_iam_member.agent_read_secret]
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_gcp_credentials.snowflake.id
}
```

## Environment variable

The variable has to be set in the agent's own runtime, where you run it, for example the agent container's environment. Its name must start with `MCD_`. If its value is encrypted with an AWS KMS key, set `kms_key_id`.

```terraform
# Snowflake credentials in an environment variable on the agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

resource "montecarlo_self_hosted_env_var_credentials" "snowflake" {
  connection_type = "snowflake"
  # Must start with MCD_.
  env_var_name = "MCD_SNOWFLAKE_CREDENTIALS"
  # Only when the value is encrypted with an AWS KMS key.
  # kms_key_id = "arn:aws:kms:us-east-1:123456789012:key/..."
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_env_var_credentials.snowflake.id
}
```

## File

The file has to exist in the agent's own runtime, where you run it, for example a file mounted into the agent container.

```terraform
# Snowflake credentials in a file on the agent.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

# The warehouse sits on the deployment whose agent reads the secret.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = "<deployment id>"
}

resource "montecarlo_self_hosted_file_credentials" "snowflake" {
  connection_type = "snowflake"
  file_path       = "/etc/monte-carlo/snowflake.json"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_self_hosted_file_credentials.snowflake.id
}
```

## Afterwards

- Monte Carlo reads the secret every time it uses the connection, so rotating it in your store needs no Terraform change.
- Monte Carlo refuses to delete credentials while a connection uses them. Terraform's graph already destroys the connection first.
