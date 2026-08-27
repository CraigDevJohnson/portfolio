variable "environment" {
  type = string

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod"
  }
}

variable "name_prefix" { type = string }

variable "aws_region" { type = string }

variable "ecr_repository_url" { type = string }

variable "image_digest" {
  type = string

  validation {
    condition     = can(regex("^sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be a sha256 digest"
  }
}

variable "lambda_memory_mb" { type = number }

variable "lambda_timeout_seconds" {
  type = number

  validation {
    condition     = var.lambda_timeout_seconds <= 29
    error_message = "lambda_timeout_seconds must be 29 seconds or less"
  }
}

variable "reserved_concurrency" {
  type = number

  validation {
    condition     = var.reserved_concurrency == -1 || var.reserved_concurrency >= 1
    error_message = "reserved_concurrency must be -1 for unreserved mode or at least 1"
  }
}

variable "log_retention_days" { type = number }

variable "enable_pitr" { type = bool }

variable "enable_deletion_protection" { type = bool }

variable "alarm_action_arns" { type = list(string) }

variable "domain_names" { type = set(string) }

variable "request_custom_domain" { type = bool }

variable "activate_custom_domain" { type = bool }

variable "live_version_override" {
  type    = number
  default = null
}
