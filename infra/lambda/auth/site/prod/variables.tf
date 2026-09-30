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
  description = "Production Google OAuth client ID, supplied privately."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_id) != ""
    error_message = "google_client_id is required."
  }
}

variable "google_client_secret" {
  description = "Production Google OAuth client secret, supplied privately."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_secret) != ""
    error_message = "google_client_secret is required."
  }
}

variable "cognito_domain_prefix" {
  description = "Globally unique production managed-login prefix. Defaults to portfolio-lambda-prod-site-<aws_account_id>; override only after a live availability review, and keep the portfolio-lambda-prod-site- prefix that the prod environment root requires."
  type        = string
  default     = null

  validation {
    condition = var.cognito_domain_prefix == null ? true : (
      length(var.cognito_domain_prefix) <= 63 &&
      can(regex("^portfolio-lambda-prod-site-[a-z0-9-]*[a-z0-9]$", var.cognito_domain_prefix))
    )
    error_message = "cognito_domain_prefix must start with portfolio-lambda-prod-site-, use at most 63 lowercase letters, digits, or hyphens, and not end with a hyphen."
  }
}
