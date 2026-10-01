# An AWS data store, end to end: the deployment, the bucket and role Monte Carlo reaches, and
# the registration that ties them together.
#
# Same ordering as the collection agent, and for the same reason. Creating the deployment
# generates an external id; the role Monte Carlo assumes has to trust it, so the deployment
# comes first and the registration reads the bucket and role back.
#
#   https://docs.getmontecarlo.com/docs/direct-connection-with-an-aws-data-store
#
# There is no published module for a data store, so the bucket and the role are defined here.

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
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # the provider documentation for the alternatives.
}

# The Monte Carlo account that assumes the role, which differs per Monte Carlo deployment. The
# value below is a placeholder. Take yours from the Account Information page in the product,
# under Collection, as "AWS account ID", and substitute it before applying.
locals {
  mcd_account_id = "590183797493"
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "AWS"
}

# A prefix rather than a fixed name: bucket names are globally unique, so a fixed one collides
# with any other copy of this example.
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

# Monte Carlo writes query output, root-cause samples and idempotency markers under their own
# prefixes. None of it is worth keeping longer than 90 days.
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

# The role Monte Carlo assumes, trusting the external id the deployment generated.
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

# The bucket-level reads matter as much as the object ones. Monte Carlo validates storage
# access when the store is registered, and a policy missing them registers and then fails
# that validation.
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

# Registering validates storage access, so it has to wait for the policy and the bucket's own
# settings. The reference to the role's ARN alone would let it run as soon as the role existed.
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
