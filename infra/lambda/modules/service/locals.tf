locals {
  function_name        = var.name_prefix
  google_table         = "${var.name_prefix}-google-connections"
  soccer_table         = "${var.name_prefix}-soccer-sessions"
  soccer_archive_table = "${var.name_prefix}-soccer-history"
  ssm_base             = "/portfolio/lambda/${var.environment}"
  ssm_names            = toset(concat(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"], var.site == null ? [] : ["SITE_SESSION_KEY"]))
  ssm_paths            = { for name in local.ssm_names : name => "${local.ssm_base}/${name}" }
  image_uri            = "${var.ecr_repository_url}@${var.image_digest}"

  # Durable collection needs the planned table, reviewed numeric limits, and
  # an explicit activation; the daily schedule also needs its own activation
  # and a reviewed expression. Any unset input leaves both off.
  history_collection_enabled = var.enable_soccer_history && var.activate_soccer_history_collection && var.soccer_history_limits != null
  history_schedule_enabled   = local.history_collection_enabled && var.activate_soccer_history_schedule && var.soccer_history_schedule_expression != null
}

locals {
  history_limit_environment = var.soccer_history_limits == null ? {} : {
    SOCCER_HISTORY_MAX_TEAMS       = tostring(var.soccer_history_limits.max_enrolled_teams)
    SOCCER_HISTORY_PLAYER_RESERVED = tostring(var.soccer_history_limits.reserved_player_slots)
    SOCCER_HISTORY_MAX_REQUESTS    = tostring(var.soccer_history_limits.max_requests_per_run)
    SOCCER_HISTORY_MAX_RETRIES     = tostring(var.soccer_history_limits.max_retries_per_team)
    SOCCER_HISTORY_MIN_INTERVAL_MS = tostring(var.soccer_history_limits.min_request_interval_ms)
  }

  # The HTTP runtime enrolls teams only once collection is activated.
  history_collection_environment = local.history_collection_enabled ? merge(local.history_limit_environment, {
    SOCCER_HISTORY_COLLECTION_ENABLED = "true"
    SOCCER_ARCHIVE_TABLE_NAME         = aws_dynamodb_table.soccer_history[0].name
  }) : {}
}

locals {
  site_environment = var.site == null ? {} : {
    SITE_SESSION_KEY          = local.ssm_paths.SITE_SESSION_KEY
    SITE_COGNITO_DOMAIN       = var.site.cognito_domain
    SITE_COGNITO_ISSUER       = var.site.cognito_issuer
    SITE_COGNITO_CLIENT_ID    = var.site.cognito_client_id
    SITE_COGNITO_REDIRECT_URI = var.site.redirect_uri
    SITE_COGNITO_LOGOUT_URI   = var.site.logout_uri
    SITE_INVITATIONS_JSON     = jsonencode({ for email, grants in var.site.invitations : email => sort(tolist(grants)) })
    SITE_ALLOW_LOCAL_CALLBACK = tostring(var.site.allow_local_callback)
  }

  # The portal follows site sign-in and the management grant; the switch
  # carries only the region its read-only EC2 and metric calls use.
  management_environment = var.management == null ? {} : {
    MGMT_AWS_REGION = var.management.aws_region
  }
}
