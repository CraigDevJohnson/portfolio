variable "environment" { type = string }

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

variable "lambda_timeout_seconds" { type = number }

variable "reserved_concurrency" { type = number }

variable "log_retention_days" { type = number }

variable "enable_pitr" { type = bool }

variable "enable_deletion_protection" { type = bool }

variable "alarm_action_arns" {
  type = list(string)

  validation {
    condition = length(var.alarm_action_arns) > 0 && alltrue([
      for arn in var.alarm_action_arns : can(regex("^arn:[^:]+:[^:]+:[^:]*:[^:]*:.+$", arn))
    ])
    error_message = "alarm_action_arns must contain at least one ARN"
  }
}

variable "domain_names" { type = set(string) }

variable "request_custom_domain" { type = bool }

variable "activate_custom_domain" { type = bool }

variable "live_version_override" {
  type    = number
  default = null
}
