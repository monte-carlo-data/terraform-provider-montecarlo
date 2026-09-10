# A GCP data store, end to end: the deployment, the bucket and service account Monte Carlo
# reaches, and the registration that ties them together.
#
#   https://docs.getmontecarlo.com/docs/deployment-and-connecting
#
# There is no published module for a data store, so the bucket, role, service account and key are
# defined here, following Monte Carlo's template in mcd-public-resources
# (templates/terraform/gcp_data_store). No external id on GCP, so only the registration depends
# on the deployment.

terraform {
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
  # Credentials come from the environment or from the Monte Carlo CLI's profile. See
  # ../../provider/provider.tf for the alternatives.
}

resource "montecarlo_deployment" "data_store" {
  name             = "production"
  type             = "COLLECTION_DATA_STORE"
  runtime_platform = "GCP"
}

# Suffixed rather than fixed: bucket names are globally unique, so a fixed one collides with
# any other copy of this example.
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

# The bucket-level reads matter as much as the object ones: the collector's storage validation
# exercises them, so a role missing them registers and then fails validation.
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

# The key arrives base64-encoded from the google provider; the API takes the key file's
# contents. The registration waits for the IAM binding, not only the key: the collector
# validates storage access during registration.
resource "montecarlo_gcp_collection_data_store" "store" {
  deployment_id       = montecarlo_deployment.data_store.id
  bucket_name         = google_storage_bucket.store.name
  service_account_key = base64decode(google_service_account_key.store.private_key)

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
