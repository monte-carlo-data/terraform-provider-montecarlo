# terraform-provider-montecarlo

The official Terraform provider for Monte Carlo. It manages the deployments, collection agents
and collection data stores that connect your environment to
[Monte Carlo](https://docs.getmontecarlo.com/docs/platform-architecture), and the warehouses,
credentials and connections that run through them.

## Status

Early, and **not yet published to the Terraform Registry**, so `terraform init` against the
source below does not resolve yet. Until it is published the provider has to be built and run
locally, which takes two commands — see
[AGENTS.md](AGENTS.md#running-a-locally-built-provider).

> There are community providers with the same name. This one will be published as
> `monte-carlo-data/montecarlo` — check the source when adding it.

## Usage

The configuration below is
[`examples/resources/montecarlo_aws_collection_agent/`](examples/resources/montecarlo_aws_collection_agent)
without its comments and
outputs, so that directory can be run as-is. Once published, the `required_providers` block is
all it needs; today it also needs the `dev_overrides` setup in
[AGENTS.md](AGENTS.md#running-a-locally-built-provider).

```hcl
terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AWS"
}

# https://registry.terraform.io/modules/monte-carlo-data/mcd-agent/aws
module "mcd_agent" {
  source  = "monte-carlo-data/mcd-agent/aws"
  version = "~> 1.0"

  region = "us-east-1"

  # The external id the deployment minted. Without the provider, this is the value you would
  # copy out of the Monte Carlo UI by hand.
  external_id = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}
```

Creating the deployment mints an external id that the agent's assumable role has to trust. So
the deployment comes first, the agent module takes that id as an input, and the agent registers
onto the deployment using the module's outputs — the three blocks above, in that order.

## Resources and data sources

The deployment, and the agents and data stores that register onto it:

| Resource | Manages |
|----------|---------|
| `montecarlo_deployment` | A deployment: what a collection agent or data store registers onto. Mints `aws_external_id`. |
| `montecarlo_aws_collection_agent` | Registration of a Lambda-backed collection agent onto an AWS deployment. |
| `montecarlo_aws_collection_data_store` | Registration of an S3 bucket as a deployment's collection data store. |
| `montecarlo_azure_collection_agent` | Registration of a Function App-backed collection agent onto an Azure deployment. |
| `montecarlo_azure_collection_data_store` | Registration of a blob container as a deployment's collection data store. |
| `montecarlo_gcp_collection_agent` | Registration of a Cloud Run-backed collection agent onto a GCP deployment. |
| `montecarlo_gcp_collection_data_store` | Registration of a GCS bucket as a deployment's collection data store. |
| `montecarlo_generic_collection_agent` | Registration of a self-hosted collection agent onto a generic deployment. |
| `montecarlo_generic_collection_agent_oauth_client` | An OAuth client a generic collection agent presents. The secret is returned once, on create. |
| `montecarlo_generic_collection_agent_token` | A token a generic collection agent presents. The secret is returned once, on create. |

The integrations that run through a deployment. A warehouse holds connections; a connection
joins a warehouse to the credentials it reads with, and takes its type from them:

| Resource | Manages |
|----------|---------|
| `montecarlo_warehouse` | A warehouse: the container its connections belong to, and the deployment they run through. |
| `montecarlo_connection` | A connection on a warehouse, reading with a given set of credentials. |
| `montecarlo_snowflake_credentials` | Snowflake key pair credentials stored by Monte Carlo. |
| `montecarlo_self_hosted_aws_credentials` | Credentials your agent reads from AWS Secrets Manager. Monte Carlo stores only where to find them. |
| `montecarlo_self_hosted_azure_credentials` | The same, from Azure Key Vault. |
| `montecarlo_self_hosted_gcp_credentials` | The same, from GCP Secret Manager. |
| `montecarlo_self_hosted_env_var_credentials` | The same, from an environment variable on the agent. |
| `montecarlo_self_hosted_file_credentials` | The same, from a file mounted into the agent. |

A self-hosted credentials resource records a reference, not a secret. The secret itself has to
exist in your store already, holding the JSON schema documented for its connection type, and
the agent has to be able to read it. See
[self-hosted credentials](https://docs.getmontecarlo.com/docs/self-hosted-credentials) for both.

Data sources read something already registered. Each is looked up by its own id attribute —
not by `id`, which is computed:

| Data source | Lookup argument |
|-------------|-----------------|
| `montecarlo_aws_collection_agent` | `collection_agent_id` |
| `montecarlo_aws_collection_data_store` | `collection_data_store_id` |
| `montecarlo_azure_collection_agent` | `collection_agent_id` |
| `montecarlo_azure_collection_data_store` | `collection_data_store_id` |
| `montecarlo_gcp_collection_agent` | `collection_agent_id` |
| `montecarlo_gcp_collection_data_store` | `collection_data_store_id` |
| `montecarlo_generic_collection_agent` | `collection_agent_id` |
| `montecarlo_generic_collection_agent_oauth_client` | `credential_id` |
| `montecarlo_generic_collection_agent_token` | `credential_id` |
| `montecarlo_warehouse` | `warehouse_id` |
| `montecarlo_connection` | `connection_id` |
| `montecarlo_snowflake_credentials` | `credentials_id` |
| `montecarlo_self_hosted_aws_credentials` | `credentials_id` |
| `montecarlo_self_hosted_azure_credentials` | `credentials_id` |
| `montecarlo_self_hosted_gcp_credentials` | `credentials_id` |
| `montecarlo_self_hosted_env_var_credentials` | `credentials_id` |
| `montecarlo_self_hosted_file_credentials` | `credentials_id` |

`terraform providers schema -json` lists every attribute of every resource and data source
above, with its description.

## Import

Every resource supports import, keyed on the `id` Monte Carlo assigned — the same id the API
and the corresponding data source return. This is the path for a deployment or agent that was
registered by hand before the provider existed:

```bash
terraform import montecarlo_deployment.agent <deployment-id>
```

Import brings the resource under management without recreating it. Run `terraform plan`
afterwards: an empty plan means the configuration matches what is registered.

## Credentials

Credentials can be set on the provider, taken from the environment, or read from a profile
written by the [Monte Carlo CLI](https://docs.getmontecarlo.com/docs/using-the-cli), in that
order. If you have run the CLI, the provider works with nothing configured at all — the
endpoint comes from the profile too:

```hcl
provider "montecarlo" {}
```

The profile is a section of `~/.mcd/profiles.ini`, shared with the CLI and the Python SDK, so
credentials are configured once per machine.

| Provider attribute | Environment variable | Profile key |
|--------------------|----------------------|-------------|
| `endpoint` | — | `mcd_api_endpoint` |
| `token_id` | `MCD_DEFAULT_API_ID` | `mcd_id` |
| `token_secret` | `MCD_DEFAULT_API_TOKEN` | `mcd_token` |
| `client_id` | `MCD_DEFAULT_OAUTH_CLIENT_ID` | `mcd_oauth_client_id` |
| `client_secret` | `MCD_DEFAULT_OAUTH_CLIENT_SECRET` | `mcd_oauth_client_secret` |
| `instance` | `MCD_DEFAULT_INSTANCE_ID` | `mcd_instance_id` |
| `token_url` | — | `mcd_token_endpoint` |
| `profile` | `MCD_DEFAULT_PROFILE` | — |

`token_id`/`token_secret` are an API token pair, and `client_id`/`client_secret` are OAuth
client credentials, which also need `instance`. Set one mechanism or the other, not both. See
[API authentication](https://docs.getmontecarlo.com/docs/api-authentication) for how to obtain
either.

If you do put credentials in your configuration, keep them in variables and the values in a
`*.tfvars` file — never a literal in a committed `.tf` file.

## Examples

- [`examples/provider/`](examples/provider) — the provider block, and the credential alternatives.
- [`examples/resources/`](examples/resources) — one directory per resource, each a complete
  configuration: the deployment, the cloud resources where the platform has any, and the
  registration. The collection agents on AWS, Azure and GCP use the published agent modules;
  the data stores define their bucket and role inline; the generic agent mints the credential
  the agent presents (a [token](examples/resources/montecarlo_generic_collection_agent_token) or
  an [OAuth client](examples/resources/montecarlo_generic_collection_agent_oauth_client)) and
  registers the agent you run with it.

For an integration, [`montecarlo_connection`](examples/resources/montecarlo_connection) is the
one to read first: it carries the whole chain, from the deployment through to the connection.
The others narrow it — [`montecarlo_warehouse`](examples/resources/montecarlo_warehouse) stops
at the warehouse, and the two credentials examples cover the alternatives for where the secret
lives, Monte Carlo managed
([`montecarlo_snowflake_credentials`](examples/resources/montecarlo_snowflake_credentials)) or
your own store
([`montecarlo_self_hosted_aws_credentials`](examples/resources/montecarlo_self_hosted_aws_credentials)).
The self-hosted credentials resources differ only in where the secret is read from, so the AWS
one reads across to the others. A resource with no directory here has no example yet.
