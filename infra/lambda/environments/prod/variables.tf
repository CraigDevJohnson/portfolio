variable "aws_account_id" {
  description = "Workloads account that owns the portfolio. The provider refuses credentials for any other account."
  type        = string
  default     = "793680745829"

  validation {
    condition     = can(regex("^[0-9]{12}$", var.aws_account_id))
    error_message = "aws_account_id must be a 12-digit AWS account ID."
  }
}

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
  description = "Production alarms notify only the workloads us-west-2 alerts topic that aws-setup owns."
  type        = list(string)

  validation {
    condition     = var.alarm_action_arns == tolist(["arn:aws:sns:us-west-2:${var.aws_account_id}:alerts"])
    error_message = "alarm_action_arns must be exactly the workloads us-west-2 alerts topic ARN."
  }
}

variable "domain_names" { type = set(string) }

variable "request_custom_domain" { type = bool }

variable "activate_custom_domain" {
  description = "Create the API Gateway custom domains. Pass false to plan the environment on its execute-api endpoint while the hostname still belongs to another account."
  type        = bool
}

variable "live_version_override" {
  type    = number
  default = null
}

variable "site" {
  description = "Reviewed non-secret production site identity: the site auth root's site_runtime fields plus invitations, the only reviewed production grant map. Production grants only soccer."
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

  # Decision 6 (2026-09-30): the production Lambda role has no EC2 or metric
  # grants, so a management grant would only open a portal that fails.
  validation {
    condition     = var.site == null ? true : alltrue([for grants in values(var.site.invitations) : !contains(grants, "management")])
    error_message = "Production site.invitations may grant only soccer: production has no management portal EC2 or metric grants, so a management grant is refused (decision 6)."
  }
}

# LPS history sync (#80). Each is required, so its reviewed value lives in
# prod.auto.tfvars and a saved plan's apply sees the same input; never pass
# them with -var. The service module validates them and plans nothing for
# history until each stage's switches and inputs are all set.
variable "enable_soccer_history" {
  description = "Plan the durable Soccer history table and the HTTP runtime's grant to it."
  type        = bool
}

variable "soccer_history_limits" {
  description = "Reviewed source-use and cost ceilings for history collection and the daily worker."
  type = object({
    max_enrolled_teams      = number
    reserved_player_slots   = number
    max_requests_per_run    = number
    max_retries_per_team    = number
    min_request_interval_ms = number
    worker_timeout_seconds  = number
  })
}

variable "activate_soccer_history_collection" {
  description = "Let the HTTP runtime enroll teams into durable history."
  type        = bool
}

variable "activate_soccer_history_schedule" {
  description = "Plan the daily history worker, its schedule, failure queue and alarms. Applying it starts live LPS polling."
  type        = bool
}

variable "soccer_history_schedule_expression" {
  description = "Reviewed once-daily UTC EventBridge Scheduler expression, or null for no schedule."
  type        = string
}
