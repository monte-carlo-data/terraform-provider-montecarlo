---
page_title: "Import a connection"
description: |-
  Bring a warehouse and connection created in the Monte Carlo app under Terraform, then have Terraform manage the connection's credentials.
---

# Import a connection

A connection created in the Monte Carlo app can be brought under Terraform without recreating it. This guide uses MySQL. The same steps apply to any other connection type, with that type's credentials resource in place of `montecarlo_mysql_credentials`.

1. Import the warehouse and the connection as they are. The connection keeps the credentials it was created with.
2. Optionally, and recommended, give the connection credentials that Terraform manages. From then on its credentials change only through Terraform.

## Before you start

- The official provider, `monte-carlo-data/montecarlo`, at v0.1.34 or later. Earlier versions replace the connection when `credentials_id` changes. A registry search for Monte Carlo also returns community providers, whose resources and import ids are different.
- Terraform 1.11 or later, for write-only arguments.
- The provider configured with an API token or OAuth client credentials. See [Authenticate the provider](authenticate-the-provider).
- The ids of the warehouse, its deployment and the connection. In the Monte Carlo app, open **Guides & support** (the **?** icon), choose **API explorer**, and on the **REST** tab run `GET /api/v2/connections`. Each connection is listed with its `id` and `name`, its warehouse's `warehouse_id` and `warehouse_name`, and its `deployment_id`. With the Monte Carlo CLI, `montecarlo connections list` prints the same.

## Import the warehouse and the connection

Write an `import` block and a matching resource for each, with the names and deployment they have in Monte Carlo.

```terraform
# Bring a MySQL warehouse and connection created in the Monte Carlo app under
# Terraform, unchanged. The first plan imports both and changes nothing.

terraform {
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

import {
  to = montecarlo_warehouse.mysql
  id = "<warehouse id>"
}

import {
  to = montecarlo_connection.mysql
  id = "<connection id>"
}

# `type`, not `connection_type`: import leaves `connection_type` unset, and
# setting it replaces the warehouse.
resource "montecarlo_warehouse" "mysql" {
  name          = "production-mysql"
  type          = "mysql"
  deployment_id = "<deployment id>"
}

# No `credentials_id` yet: a connection created in the app has none.
resource "montecarlo_connection" "mysql" {
  name         = "production-mysql"
  warehouse_id = montecarlo_warehouse.mysql.id
}
```

Two arguments need care:

- On the warehouse, set `type = "mysql"`, not `connection_type`. Import leaves `connection_type` unset, and setting it replaces the warehouse.
- On the connection, leave `credentials_id` out. A connection created in the app has no credentials resource behind it, so there is nothing to point it at yet.

`terraform plan` should report two resources to import and nothing to add, change or destroy. Apply it, and the next plan is empty.

Instead of writing the resource blocks, you can keep only the `import` blocks and run `terraform plan -generate-config-out=generated.tf`. Terraform writes the resources for you, with `type = "mysql"`, `connection_type = null` and `credentials_id = null`.

## Manage the credentials with Terraform

Imported as above, the connection still reads with the credentials it was created with, and those can still be edited in the Monte Carlo app. To manage them in Terraform instead, create a credentials resource and point the connection's `credentials_id` at it.

```terraform
# The imported MySQL connection, pointed at credentials Terraform manages. The
# connection is updated in place.

terraform {
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

import {
  to = montecarlo_warehouse.mysql
  id = "<warehouse id>"
}

import {
  to = montecarlo_connection.mysql
  id = "<connection id>"
}

resource "montecarlo_warehouse" "mysql" {
  name          = "production-mysql"
  type          = "mysql"
  deployment_id = "<deployment id>"
}

# Pass it as TF_VAR_mysql_password. Ephemeral, so it is never stored in state
# or a plan.
variable "mysql_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "montecarlo_mysql_credentials" "mysql" {
  host = "mysql.example.com"
  port = 3306
  user = "monte_carlo"

  # Changing the password alone plans nothing: bump the version with it.
  password_wo         = var.mysql_password
  password_wo_version = 1
}

resource "montecarlo_connection" "mysql" {
  name           = "production-mysql"
  warehouse_id   = montecarlo_warehouse.mysql.id
  credentials_id = montecarlo_mysql_credentials.mysql.id
}
```

The credentials have to be of the connection's type: `montecarlo_mysql_credentials` for a MySQL connection. `terraform plan` creates the credentials and updates the connection in place. Pass the password as `TF_VAR_mysql_password` when you apply.

Once a connection has `credentials_id`, the Monte Carlo app refuses to edit its credentials and says they are managed through the Monte Carlo REST API. Change them in Terraform, the REST API or the Monte Carlo CLI. To rotate the password, change it and bump `password_wo_version`.

### Self-hosted credentials

The same change moves a connection between credentials Monte Carlo stores and self-hosted credentials, which keep the secret in your own store, such as `montecarlo_self_hosted_aws_credentials`. Point `credentials_id` at the other resource, and Terraform updates the connection in place again. Self-hosted credentials are read by your collection agent, so the connection's deployment needs one; on a deployment without an agent, Monte Carlo refuses the change and the connection keeps its credentials. See [Use self-hosted credentials](use-self-hosted-credentials).

## Afterwards

- The warehouse and the connection are now managed like any other resource. `terraform destroy`, or removing a resource block, deletes it in Monte Carlo. To stop managing one without deleting it, replace its resource block with a `removed` block whose `lifecycle` sets `destroy = false`.
- The `import` blocks have done their job once applied. You can delete them, or keep them: an import block for a resource already in state does nothing.
