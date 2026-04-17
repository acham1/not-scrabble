variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "not-scrabble"
}

variable "region" {
  description = "GCP region for Cloud Run and Artifact Registry"
  type        = string
  default     = "us-central1"
}

variable "image_tag" {
  description = "Container image tag to deploy (e.g. 'latest' or a git SHA)"
  type        = string
  default     = "latest"
}

variable "google_client_id" {
  description = "Google OAuth client ID for Sign-In (create in GCP console)"
  type        = string
}

variable "allowlist_emails" {
  description = "Comma-separated emails allowed to sign in (empty = open)"
  type        = string
  default     = ""
}

variable "vapid_public_key" {
  description = "VAPID public key for Web Push"
  type        = string
  default     = ""
}

variable "vapid_private_key" {
  description = "VAPID private key for Web Push"
  type        = string
  sensitive   = true
  default     = ""
}

variable "vapid_contact" {
  description = "VAPID contact email (e.g. mailto:you@example.com)"
  type        = string
  default     = ""
}
