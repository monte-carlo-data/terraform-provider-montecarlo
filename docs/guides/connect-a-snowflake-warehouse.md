---
page_title: "Connect a Snowflake warehouse"
description: |-
  Connect Monte Carlo to Snowflake from start to finish: a deployment, a warehouse, credentials, and the connection that joins them.
---

# Connect a Snowflake warehouse

Monte Carlo reaches Snowflake through four objects, and Terraform creates them in the order their references set:

1. A **deployment** that Monte Carlo's queries run through: one you run with a registered collection agent, or the one Monte Carlo hosts for your account.
2. A **warehouse** on that deployment (`montecarlo_warehouse`), which groups the connections for one Snowflake account.
3. **Credentials** for Monte Carlo's Snowflake user (`montecarlo_snowflake_credentials`). They depend on nothing else.
4. A **connection** (`montecarlo_connection`) that joins the warehouse and the credentials. Its type comes from the credentials, and it has to fit the warehouse's type.

## Before you start

- Terraform 1.11 or later. The private key is a write-only argument, so it is never stored in state or a plan.
- A Snowflake service user with a key pair and the grants Monte Carlo needs. See [Snowflake](https://docs.getmontecarlo.com/docs/snowflake) in the Monte Carlo docs. Key pair is the only authentication Snowflake credentials support.
- The provider configured with an API token or OAuth client credentials. See [Authenticate the provider](authenticate-the-provider).

## On a deployment you run

This configuration creates the deployment and runs Monte Carlo's agent in your AWS account with the [`mcd-agent` module](https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws). It registers the agent, then creates the warehouse, credentials and connection. The warehouse `depends_on` the agent registration: it needs only the deployment's id, but a warehouse whose agent is not registered yet has nothing to serve its connections. [Deploy a collection agent](deploy-a-collection-agent) covers Azure, GCP and other platforms.

```terraform
# A Snowflake connection, end to end: the deployment, the agent, the warehouse
# that holds the connection, the credentials it reads with, and the connection
# itself.
#
# The chain is deployment -> agent -> warehouse -> credentials -> connection,
# and Terraform derives that order from the references between the blocks. The
# two halves that meet at the connection are independent of each other: a
# warehouse says which deployment queries run through, credentials say what to
# authenticate with, and neither knows about the other until the connection
# joins them.
#
# The connection's type is not configured — it is taken from the credentials,
# and it has to fit the warehouse's type. Snowflake credentials on a `snowflake`
# warehouse, as below.
#
# For the Monte Carlo managed credentials used here, see the
# `montecarlo_snowflake_credentials` example. To keep the secret in your own
# store instead, swap that resource for one of the self-hosted ones and point
# `credentials_id` at it — the `montecarlo_self_hosted_aws_credentials` example
# is the AWS Secrets Manager version, and nothing else in this file changes.

terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
  # See the provider documentation for the alternatives.
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
# The module configures the AWS provider itself, from its own `region` variable.
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/aws"
  version = "~> 1.0"

  region           = "us-east-1"
  cloud_account_id = data.montecarlo_hosting_private_link.this.aws.account_id

  # The external id the deployment generated. This is the dependency that forces
  # the ordering.
  external_id = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}

# `depends_on` rather than a reference: the warehouse needs only the
# deployment's id, but a warehouse whose agent is not yet registered has nothing
# to serve its connections.
resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = montecarlo_deployment.agent.id

  depends_on = [montecarlo_aws_collection_agent.agent]
}

resource "montecarlo_snowflake_credentials" "snowflake" {
  account = "xy12345.us-east-1"
  user    = "MONTE_CARLO"

  # PEM text, BEGIN and END lines included. The placeholder below shows the
  # shape; in a real configuration read the key from a file kept out of version
  # control, with `file("${path.module}/snowflake_key.p8")`, rather than pasting
  # it into a .tf file that gets committed. Write-only, so it is never stored in
  # state; bump the version to rotate it.
  private_key_wo         = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MII...
    -----END PRIVATE KEY-----
  EOT
  private_key_wo_version = 1

  warehouse = "MONTE_CARLO_WH"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_snowflake_credentials.snowflake.id

  # The jobs Monte Carlo runs on the connection. Omitting it takes the defaults
  # for the connection type, which is what the Monte Carlo app does and what
  # most configurations want.
  # job_types = ["metadata", "query_logs", "sql_query"]
}

output "connection_id" {
  description = "Id of the connection."
  value       = montecarlo_connection.snowflake.id
}

output "connection_type" {
  description = "What the connection reaches, taken from the credentials rather than configured."
  value       = montecarlo_connection.snowflake.connection_type
}

output "credentials_storage_type" {
  description = "Where the secret lives. mc_managed here; a self-hosted credentials resource would report its own store."
  value       = montecarlo_connection.snowflake.credentials_storage_type
}

output "job_types" {
  description = "The jobs Monte Carlo runs on the connection, defaulted by the API when not configured."
  value       = montecarlo_connection.snowflake.job_types
}
```

## On the deployment Monte Carlo hosts

To have Monte Carlo run the queries, skip the deployment and agent. Put the warehouse on the deployment Monte Carlo hosts for your account, and pass its id in as `hosted_deployment_id`. In the Monte Carlo app, go to **Settings > Deployments**, open the deployment of type **Cloud**, and copy its **Deployment UUID**.

```terraform
terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  # Credentials come from the environment or from the Monte Carlo CLI's profile.
}

variable "hosted_deployment_id" {
  description = "Deployment UUID of the Cloud deployment under Settings > Deployments in the Monte Carlo app."
  type        = string
}

resource "montecarlo_warehouse" "snowflake" {
  name          = "production-snowflake"
  type          = "snowflake"
  deployment_id = var.hosted_deployment_id
}

resource "montecarlo_snowflake_credentials" "snowflake" {
  # The account identifier, without the .snowflakecomputing.com suffix.
  account = "xy12345.us-east-1"
  user    = "MONTE_CARLO"

  # PEM text. In a real configuration, read the key from a file kept out of
  # version control, with `file("${path.module}/snowflake_key.p8")`.
  private_key_wo         = <<-EOT
    -----BEGIN PRIVATE KEY-----
    MII...
    -----END PRIVATE KEY-----
  EOT
  private_key_wo_version = 1

  # The virtual warehouse queries run in. Omit it to use the user's default.
  warehouse = "MONTE_CARLO_WH"
}

resource "montecarlo_connection" "snowflake" {
  name           = "production-snowflake"
  warehouse_id   = montecarlo_warehouse.snowflake.id
  credentials_id = montecarlo_snowflake_credentials.snowflake.id
}
```

## Afterwards

- To rotate the key, change `private_key_wo` and bump `private_key_wo_version`. Changing the key without bumping the version plans nothing.
- On a deployment you run, you can keep the key in your own secret store instead of Monte Carlo's. Replace `montecarlo_snowflake_credentials` with a self-hosted credentials resource and point `credentials_id` at it. Terraform replaces the connection, because `credentials_id` can't change in place. Your collection agent reads self-hosted credentials, so this doesn't apply on the deployment Monte Carlo hosts. See [Use self-hosted credentials](use-self-hosted-credentials).
- `job_types` on the connection defaults to the jobs the Monte Carlo app runs for Snowflake. Set it only to run fewer.
