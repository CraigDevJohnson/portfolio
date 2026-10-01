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

# worker_timeout_seconds must cover pacing the whole request budget plus the
# slowest single team: every allowed attempt waiting out its interval and the
# worker's 15-second LPS request timeout (lpsClientTimeout in internal/app),
# the 1, 2, 4, ... second backoffs between them, and the 10 seconds the worker
# keeps to store its last team and report (dailyWrapUpAllowance in
# internal/soccerarchive). A run that still runs short of time stops starting
# teams and reports the rest as left for the next run.
variable "soccer_history_limits" {
  description = "Reviewed source-use and cost ceilings; null keeps durable enrollment and scheduling off."
  type = object({
    max_enrolled_teams      = number
    reserved_player_slots   = number
    max_requests_per_run    = number
    max_retries_per_team    = number
    min_request_interval_ms = number
    worker_timeout_seconds  = number
  })
  default = null

  validation {
    condition = var.soccer_history_limits == null ? true : (
      var.soccer_history_limits.max_enrolled_teams > 0 &&
      var.soccer_history_limits.reserved_player_slots >= 0 &&
      var.soccer_history_limits.reserved_player_slots < var.soccer_history_limits.max_enrolled_teams &&
      var.soccer_history_limits.max_requests_per_run > 0 &&
      var.soccer_history_limits.max_requests_per_run >= var.soccer_history_limits.max_enrolled_teams &&
      var.soccer_history_limits.max_retries_per_team >= 0 &&
      var.soccer_history_limits.max_retries_per_team <= 5 &&
      var.soccer_history_limits.min_request_interval_ms > 0 &&
      var.soccer_history_limits.worker_timeout_seconds >= 30 &&
      var.soccer_history_limits.worker_timeout_seconds <= 900 &&
      (var.soccer_history_limits.max_requests_per_run - 1) * var.soccer_history_limits.min_request_interval_ms +
      (1 + var.soccer_history_limits.max_retries_per_team) * (15000 + var.soccer_history_limits.min_request_interval_ms) +
      (pow(2, var.soccer_history_limits.max_retries_per_team) - 1) * 1000 + 10000 < var.soccer_history_limits.worker_timeout_seconds * 1000 &&
      alltrue([for value in values(var.soccer_history_limits) : value == floor(value)])
    )
    error_message = "soccer_history_limits must contain reviewed positive integer ceilings, bounded retry, reserve, and pacing values, and a Lambda timeout that covers pacing every request plus one team's slowest fetch."
  }
}

variable "activate_soccer_history_collection" {
  description = "Let the HTTP runtime enroll teams into durable history. Needs enable_soccer_history, reviewed limits, and an alert destination; keep false until the #80 activation review approves collection."
  type        = bool
  default     = false
}

variable "activate_soccer_history_schedule" {
  description = "Plan the daily history worker, its schedule, failure queue, and alarms. Needs collection activation and a reviewed expression; keep false until live LPS polling is approved."
  type        = bool
  default     = false
}

variable "soccer_history_schedule_expression" {
  description = "Operator-reviewed daily EventBridge Scheduler expression; unset by default."
  type        = string
  default     = null

  validation {
    condition     = var.soccer_history_schedule_expression == null ? true : can(regex("^cron\\(([0-5]?[0-9]) ([01]?[0-9]|2[0-3]) \\* \\* \\? \\*\\)$", var.soccer_history_schedule_expression))
    error_message = "soccer_history_schedule_expression must be a once-daily UTC cron expression."
  }
}

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
  description = "Identity-free development portal switch: null, or the region where the portal may read EC2 inventory and metrics. Setting it grants those read-only actions and passes MGMT_AWS_REGION; the portal signs in through site identity and the management grant, so it carries no Cognito, callback, allowlist or session settings."
  type = object({
    aws_region = string
  })
  default = null

  validation {
    condition     = var.management == null ? true : var.management.aws_region == "us-west-2"
    error_message = "management must be null or exactly { aws_region = \"us-west-2\" }."
  }
}
