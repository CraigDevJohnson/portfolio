variable "environment" {
  type     = string
  nullable = false

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod."
  }
}

variable "google_client_id" {
  description = "Environment-specific Google OAuth client ID for Cognito federation."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_id) != ""
    error_message = "google_client_id must be supplied privately."
  }
}

variable "google_client_secret" {
  description = "Environment-specific Google OAuth client secret for Cognito federation."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = trimspace(var.google_client_secret) != ""
    error_message = "google_client_secret must be supplied privately."
  }

  # Google issues GOCSPX- secrets with 28 more characters. Anything else, such
  # as a value pasted twice, plans cleanly but makes every sign-in fail.
  validation {
    condition     = can(regex("^GOCSPX-[A-Za-z0-9_-]{28}$", var.google_client_secret))
    error_message = "google_client_secret must be exactly one Google client secret: GOCSPX- followed by 28 letters, digits, hyphens or underscores."
  }
}

variable "cognito_domain_prefix" {
  description = "Environment-specific, globally unique Cognito managed-login prefix."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$", var.cognito_domain_prefix))
    error_message = "cognito_domain_prefix must be 1-63 lowercase letters, digits, or hyphens and cannot start or end with a hyphen."
  }
}

variable "enable_local_callback" {
  description = "Allow the development-only loopback callback after explicit review."
  type        = bool
  default     = false
  nullable    = false

  validation {
    condition     = var.environment == "dev" || !var.enable_local_callback
    error_message = "production cannot register a loopback callback."
  }
}
