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

variable "management" {
  description = "Reviewed public development management settings; never provider or session credentials."
  type = object({
    cognito_domain           = string
    cognito_issuer           = string
    cognito_client_id        = string
    redirect_uri             = string
    logout_uri               = string
    allowed_emails           = set(string)
    allow_local_callback     = bool
    ec2_management_tag_key   = string
    ec2_management_tag_value = string
  })
  default = null

  validation {
    condition = var.management == null ? true : (
      can(regex("^https://[a-z0-9-]+\\.auth\\.us-west-2\\.amazoncognito\\.com$", var.management.cognito_domain)) &&
      can(regex("^https://cognito-idp\\.us-west-2\\.amazonaws\\.com/us-west-2_[A-Za-z0-9]+$", var.management.cognito_issuer)) &&
      can(regex("^[a-z0-9]{1,128}$", var.management.cognito_client_id)) &&
      var.management.redirect_uri == "https://dev.craigdevjohnson.com/callback" &&
      var.management.logout_uri == "https://dev.craigdevjohnson.com/login" &&
      var.management.allowed_emails == toset(["craigdevjohnson@gmail.com"]) &&
      var.management.allow_local_callback != null &&
      var.management.ec2_management_tag_key == "PortfolioManagement" &&
      var.management.ec2_management_tag_value == "dev"
    )
    error_message = "management must contain only the reviewed development public identity, callbacks, allowlist and EC2 tag."
  }
}
