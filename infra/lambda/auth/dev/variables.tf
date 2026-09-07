variable "google_client_id" {
  description = "Dedicated Google OAuth client ID for Cognito federation."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_id) != ""
    error_message = "google_client_id must be provided through the private deployment channel."
  }
}

variable "google_client_secret" {
  description = "Dedicated Google OAuth client secret for Cognito federation."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_secret) != ""
    error_message = "google_client_secret must be provided through the private deployment channel."
  }
}

variable "enable_local_callback" {
  description = "Register the loopback callback used for explicit local development."
  type        = bool
  default     = false
  nullable    = false
}

variable "cognito_domain_prefix" {
  description = "Globally unique managed-login domain prefix; override only after a live availability review."
  type        = string
  default     = "portfolio-lambda-dev-mgmt-180294223248"
  nullable    = false

  validation {
    condition     = can(regex("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$", var.cognito_domain_prefix))
    error_message = "cognito_domain_prefix must be 1-63 lowercase letters, digits, or hyphens and cannot start or end with a hyphen."
  }
}
