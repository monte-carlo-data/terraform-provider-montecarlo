# terraform-provider-montecarlo

The official Terraform provider for Monte Carlo.

> There are community providers with the same name. This one is published as
> `monte-carlo-data/montecarlo` — check the source when adding it.

## Usage

```hcl
terraform {
  required_providers {
    montecarlo = {
      source = "monte-carlo-data/montecarlo"
    }
  }
}

provider "montecarlo" {
  endpoint     = "https://api.getmontecarlo.com"
  token_id     = var.mcd_id
  token_secret = var.mcd_token
}

resource "montecarlo_deployment" "agent" {
  name             = "production"
  type             = "COLLECTION_AGENT"
  runtime_platform = "AWS"
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.function_arn
  role_arn            = module.mcd_agent.invoker_role_arn
}
```

Provisioning a deployment returns an external id that the agent's trust policy has to carry,
so the deployment comes first and the agent registers onto it.

### Credentials

Credentials can be set on the provider, taken from the environment, or read from a profile
written by the Monte Carlo CLI, in that order. If you have run the CLI, the provider works
with no credentials in your configuration:

```hcl
provider "montecarlo" {
  endpoint = "https://api.getmontecarlo.com"
}
```

`terraform providers schema -json` lists every attribute with its description, including
which environment variable and profile key each falls back to.

## Status

Early. Not yet published to the Terraform Registry, so it has to be built and run locally —
see [AGENTS.md](AGENTS.md).
