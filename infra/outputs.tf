output "service_url" {
  description = "Cloud Run service URL"
  value       = google_cloud_run_v2_service.app.uri
}

output "bucket_name" {
  description = "GCS bucket for game state"
  value       = google_storage_bucket.state.name
}

output "image" {
  description = "Full container image path (without tag)"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app.repository_id}/not-scrabble"
}

output "service_account" {
  description = "Cloud Run service account email"
  value       = google_service_account.app.email
}

output "google_client_id" {
  description = "OAuth client ID for Google Sign-In"
  value       = var.google_client_id
}
