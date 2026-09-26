variable "aws_account_id" {
  description = "Workloads account that owns the portfolio. The provider refuses credentials for any other account."
  type        = string
  default     = "793680745829"

  validation {
    condition     = can(regex("^[0-9]{12}$", var.aws_account_id))
    error_message = "aws_account_id must be a 12-digit AWS account ID."
  }
}

variable "google_client_id" {
  description = "Development Google OAuth client ID, supplied privately."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_id) != ""
    error_message = "google_client_id is required."
  }
}

variable "google_client_secret" {
  description = "Development Google OAuth client secret, supplied privately."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_secret) != ""
    error_message = "google_client_secret is required."
  }
}

variable "cognito_domain_prefix" {
  description = "Globally unique development managed-login prefix. Defaults to portfolio-lambda-dev-site-<aws_account_id>; override only after a live availability review."
  type        = string
  default     = null
}

variable "enable_local_callback" {
  description = "Register the development loopback callback only after explicit opt-in."
  type        = bool
  default     = false
  nullable    = false
}
