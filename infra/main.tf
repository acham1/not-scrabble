terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# ---------------------------------------------------------------------------
# APIs
# ---------------------------------------------------------------------------
locals {
  apis = [
    "run.googleapis.com",
    "storage.googleapis.com",
    "artifactregistry.googleapis.com",
    "iam.googleapis.com",
  ]
}

resource "google_project_service" "apis" {
  for_each           = toset(local.apis)
  service            = each.value
  disable_on_destroy = false
}

# ---------------------------------------------------------------------------
# Artifact Registry — container images
# ---------------------------------------------------------------------------
resource "google_artifact_registry_repository" "app" {
  repository_id = "not-scrabble"
  location      = var.region
  format        = "DOCKER"
  description   = "Container images for not-scrabble"

  depends_on = [google_project_service.apis]
}

# ---------------------------------------------------------------------------
# GCS bucket — game state
# ---------------------------------------------------------------------------
resource "google_storage_bucket" "state" {
  name     = "${var.project_id}-state"
  location = var.region

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = false

  versioning {
    enabled = false
  }

  lifecycle_rule {
    condition {
      age = 365
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.apis]
}

# ---------------------------------------------------------------------------
# Service account — least-privilege identity for Cloud Run
# ---------------------------------------------------------------------------
resource "google_service_account" "app" {
  account_id   = "not-scrabble-run"
  display_name = "not-scrabble Cloud Run"

  depends_on = [google_project_service.apis]
}

resource "google_storage_bucket_iam_member" "app_state" {
  bucket = google_storage_bucket.state.name
  role   = "roles/storage.objectUser"
  member = "serviceAccount:${google_service_account.app.email}"
}

# ---------------------------------------------------------------------------
# Session secret — generated once, stored in Terraform state
# ---------------------------------------------------------------------------
resource "random_id" "session_secret" {
  byte_length = 32
}

# ---------------------------------------------------------------------------
# Cloud Run service
# ---------------------------------------------------------------------------
locals {
  image = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}/not-scrabble:${var.image_tag}"
}

resource "google_cloud_run_v2_service" "app" {
  name     = "not-scrabble"
  location = var.region

  deletion_protection = false

  template {
    service_account = google_service_account.app.email

    scaling {
      min_instance_count = 0
      max_instance_count = 2
    }

    containers {
      image = local.image

      ports {
        container_port = 8080
      }

      env {
        name  = "BUCKET_NAME"
        value = google_storage_bucket.state.name
      }

      env {
        name  = "GOOGLE_CLIENT_ID"
        value = var.google_client_id
      }

      env {
        name  = "SESSION_SECRET"
        value = random_id.session_secret.hex
      }

      dynamic "env" {
        for_each = var.allowlist_emails != "" ? [1] : []
        content {
          name  = "ALLOWLIST_EMAILS"
          value = var.allowlist_emails
        }
      }

      dynamic "env" {
        for_each = var.vapid_public_key != "" ? [1] : []
        content {
          name  = "VAPID_PUBLIC_KEY"
          value = var.vapid_public_key
        }
      }

      dynamic "env" {
        for_each = var.vapid_private_key != "" ? [1] : []
        content {
          name  = "VAPID_PRIVATE_KEY"
          value = var.vapid_private_key
        }
      }

      dynamic "env" {
        for_each = var.vapid_contact != "" ? [1] : []
        content {
          name  = "VAPID_CONTACT"
          value = var.vapid_contact
        }
      }

      resources {
        limits = {
          cpu    = "1"
          memory = "256Mi"
        }
        cpu_idle          = true
        startup_cpu_boost = true
      }
    }
  }

  depends_on = [google_project_service.apis]
}

# Allow unauthenticated access (public web app)
resource "google_cloud_run_v2_service_iam_member" "public" {
  name     = google_cloud_run_v2_service.app.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
