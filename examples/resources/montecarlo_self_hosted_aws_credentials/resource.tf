# Credentials kept in your own AWS Secrets Manager, which the collection agent reads at
# runtime. Monte Carlo stores only where to find them.
#
# Credentials do nothing until a connection reads with them — see
# ../montecarlo_connection/resource.tf for the whole flow. For the alternative, where Monte
# Carlo holds the secret, see ../montecarlo_snowflake_credentials/resource.tf.
#
# Two things have to be true before this works, and neither is expressible here:
#
# 1. The secret's value is the JSON schema documented for the connection type. For Snowflake
#    that is the object below, and note `private_key` is the PKCS#8 base64 body — the `MII...`
#    text, without the BEGIN and END lines the Monte Carlo managed resource takes:
#
#      {"connect_args": {"user": "MONTE_CARLO", "private_key": "MII...",
#                        "account": "xy12345.us-east-1", "warehouse": "MONTE_CARLO_WH"}}
#
#    Every supported connection type's schema is at
#    https://docs.getmontecarlo.com/docs/self-hosted-credentials
#
# 2. The agent can read the secret. The agent module grants its execution role no Secrets
#    Manager access, so the policy below adds it, scoped to this one secret. Without it the
#    connection is created and then fails to reach Snowflake.

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
  endpoint = "https://api.getmontecarlo.com"
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # ../../provider/provider.tf for the alternatives.
}

provider "aws" {
  region = "us-east-1"
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

  region      = "us-east-1"
  external_id = montecarlo_deployment.agent.aws_external_id
}

resource "montecarlo_aws_collection_agent" "agent" {
  deployment_id       = montecarlo_deployment.agent.id
  lambda_function_arn = module.mcd_agent.mcd_agent_function_arn
  role_arn            = module.mcd_agent.mcd_agent_invoker_role_arn
}

# The secret itself is assumed to exist already, holding the JSON described above. Replace the
# ARN with yours.
locals {
  snowflake_secret_arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:monte-carlo/snowflake-AbCdEf"
}

# Point 2 above: the agent reads the secret as itself, so its execution role needs the grant.
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
  # Fixes what these credentials are for, and so the type of any connection using them.
  connection_type = "snowflake"

  # Name or ARN of the secret.
  aws_secret = local.snowflake_secret_arn

  # Region of the secret. Omit it to use the deployment's own region.
  # aws_region = "us-east-1"

  # Set these only when the agent cannot read the secret as itself — for a secret in another
  # account, say. The role named here has to carry the tag MonteCarloData = "", because that is
  # the only condition under which the agent module lets its execution role assume a role.
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
