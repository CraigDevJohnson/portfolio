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
  description = "Globally unique managed-login domain prefix. Defaults to portfolio-lambda-dev-mgmt-<aws_account_id>; override only after a live availability review."
  type        = string
  default     = null

  validation {
    condition     = var.cognito_domain_prefix == null || can(regex("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$", var.cognito_domain_prefix))
    error_message = "cognito_domain_prefix must be 1-63 lowercase letters, digits, or hyphens and cannot start or end with a hyphen."
  }
}
