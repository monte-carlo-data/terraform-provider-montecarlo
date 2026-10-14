---
page_title: "Authenticate the provider"
description: |-
  Give the Monte Carlo provider an API token or OAuth client credentials: in the configuration, from the environment, or from the Monte Carlo CLI's profile.
---

# Authenticate the provider

The provider authenticates with one of three credentials. See [API authentication](https://docs.getmontecarlo.com/docs/api-authentication) in the Monte Carlo docs for how to get an API token or OAuth client credentials.

- An **API token**: `token_id` and `token_secret`.
- **OAuth client credentials**: `client_id` and `client_secret`, plus the `instance` they belong to.
- A **bearer token** obtained elsewhere: `token`. Set it in the configuration only. It has no environment variable or profile key.

Set one, not several. If you do, OAuth wins over an API token, and an API token wins over a bearer token. Half a pair in the configuration or in a profile is an error. In the environment it is ignored, and the provider moves on to the profile. The other half is never filled in from anywhere else.

The endpoint defaults to `https://api.getmontecarlo.com`, which serves every instance. Set `endpoint` only to reach a different one.

## Where credentials come from

The provider takes the first credential it finds, in this order:

1. The provider configuration.
2. Environment variables.
3. A profile in `~/.mcd/profiles.ini`, the file the [Monte Carlo CLI](https://docs.getmontecarlo.com/docs/using-the-cli) writes.

It picks a whole credential, not field by field: an API token set in the configuration is used as is, even when OAuth variables are also set in the environment.

When `endpoint` isn't set, it comes from the profile's `mcd_api_endpoint` if the profile has one, even when the credential comes from the configuration or the environment. Otherwise it is the default.

| Provider attribute | Environment variable | Profile key |
|---|---|---|
| `token_id` | `MCD_DEFAULT_API_ID` | `mcd_id` |
| `token_secret` | `MCD_DEFAULT_API_TOKEN` | `mcd_token` |
| `client_id` | `MCD_DEFAULT_OAUTH_CLIENT_ID` | `mcd_oauth_client_id` |
| `client_secret` | `MCD_DEFAULT_OAUTH_CLIENT_SECRET` | `mcd_oauth_client_secret` |
| `instance` | `MCD_DEFAULT_INSTANCE_ID` | `mcd_instance_id` |
| `endpoint` | none | `mcd_api_endpoint` |
| `token` | none | none |
| `token_url` | none | `mcd_token_endpoint` |
| `profile` | `MCD_DEFAULT_PROFILE` | none |

## In the configuration

Keep credentials in variables, and set the values from a `*.tfvars` file you don't commit or from `TF_VAR_*` environment variables. Never put them in a `.tf` file as literals.

An API token:

```terraform
terraform {
  # Write-only arguments need Terraform 1.11 or later.
  required_version = ">= 1.11"

  required_providers {
    montecarlo = {
      source  = "monte-carlo-data/montecarlo"
      version = "~> 0.1.0"
    }
  }
}

# Credentials may be set here, taken from the environment, or read from a
# profile written by the Monte Carlo CLI, in that order. Keep them in variables,
# with the values in a *.tfvars file that is not committed — never as literals
# in a .tf file.
provider "montecarlo" {
  token_id     = var.mcd_id
  token_secret = var.mcd_token
}

# If you have run the Monte Carlo CLI, both can be left out: the token comes
# from ~/.mcd/profiles.ini, and this is the whole provider block.
#
#   provider "montecarlo" {}

variable "mcd_id" {
  description = "Monte Carlo API token id. Falls back to MCD_DEFAULT_API_ID, then to the profile's mcd_id."
  type        = string
  default     = null
}

variable "mcd_token" {
  description = "Monte Carlo API token secret. Falls back to MCD_DEFAULT_API_TOKEN, then to the profile's mcd_token."
  type        = string
  sensitive   = true
  default     = null
}
```

OAuth client credentials:

```terraform
# OAuth client credentials. Keep the values in variables, set from a *.tfvars
# file that isn't committed, never as literals in a .tf file.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  client_id     = var.mcd_client_id
  client_secret = var.mcd_client_secret
  # The instance the client belongs to, such as us1.
  instance = var.mcd_instance
}

variable "mcd_client_id" {
  type = string
}

variable "mcd_client_secret" {
  type      = string
  sensitive = true
}

variable "mcd_instance" {
  type = string
}
```

## From the environment or the Monte Carlo CLI

With credentials in the environment, the provider block can be empty:

```shell
export MCD_DEFAULT_API_ID="..."
export MCD_DEFAULT_API_TOKEN="..."
```

The same empty block works on a machine where you have run the Monte Carlo CLI's `configure`, because the provider reads the CLI's `default` profile:

```terraform
# Credentials from MCD_DEFAULT_* environment variables, or from the Monte Carlo
# CLI's profile, so the provider block is empty.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {}
```

## A named profile

To use another profile, name it with `profile`, or set `MCD_DEFAULT_PROFILE`. A profile named in the configuration has to exist. The provider doesn't follow the profile the CLI's `profile use` selects: it reads `MCD_DEFAULT_PROFILE`, then `default`.

```terraform
# A named profile from ~/.mcd/profiles.ini, written by the Monte Carlo CLI.

terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  profile = "staging"
}
```
