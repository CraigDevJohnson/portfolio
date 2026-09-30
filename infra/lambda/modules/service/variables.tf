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

variable "enable_soccer_history" {
  description = "Plan the durable Soccer history table and its runtime access. Keep false until the #80 activation review approves the AWS resources."
  type        = bool
  default     = false
}

variable "alarm_action_arns" { type = list(string) }

variable "domain_names" { type = set(string) }

variable "request_custom_domain" { type = bool }

variable "activate_custom_domain" { type = bool }

variable "live_version_override" {
  type    = number
  default = null
}

variable "site" {
  description = "Reviewed public site identity and page grants for this environment; no OAuth or session secrets."
  type = object({
    cognito_domain       = string
    cognito_issuer       = string
    cognito_client_id    = string
    redirect_uri         = string
    logout_uri           = string
    invitations          = map(set(string))
    allow_local_callback = bool
  })
  default = null

  validation {
    condition = var.site == null ? true : (
      can(regex("^https://portfolio-lambda-${var.environment}-site-[a-z0-9-]+\\.auth\\.us-west-2\\.amazoncognito\\.com$", var.site.cognito_domain)) &&
      can(regex("^https://cognito-idp\\.us-west-2\\.amazonaws\\.com/us-west-2_[A-Za-z0-9]+$", var.site.cognito_issuer)) &&
      can(regex("^[a-z0-9]{1,128}$", var.site.cognito_client_id)) &&
      var.site.redirect_uri == (var.environment == "dev" ? "https://dev.craigdevjohnson.com/auth/callback" : "https://craigdevjohnson.com/auth/callback") &&
      var.site.logout_uri == (var.environment == "dev" ? "https://dev.craigdevjohnson.com/sign-in" : "https://craigdevjohnson.com/sign-in") &&
      length(var.site.invitations) > 0 &&
      alltrue([for email, grants in var.site.invitations : (
        email == lower(trimspace(email)) &&
        can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", email)) &&
        alltrue([for grant in grants : contains(["soccer", "management"], grant)])
      )]) &&
      (var.environment == "dev" || !var.site.allow_local_callback)
    )
    error_message = "site must contain this environment's public Cognito settings and reviewed invitations only."
  }
}

variable "management" {
  description = "Reviewed public development management settings; never provider or session credentials. Setting it grants the portal read-only EC2 inventory and metrics in us-west-2 and passes MGMT_AWS_REGION; Lambda receives none of the retired management-only identity fields."
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
