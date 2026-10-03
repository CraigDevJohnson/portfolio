environment            = "prod"
name_prefix            = "portfolio-lambda-prod"
aws_region             = "us-west-2"
lambda_memory_mb       = 512
lambda_timeout_seconds = 29
# Temporarily unreserved (-1): the workloads account's Lambda concurrency
# limit is still 10 while the quota increase is pending, and Lambda keeps 100
# unreserved, so a reservation of 10 can't be applied. The account limit caps
# prod at 10 meanwhile. Set this back to 10 once the limit is at least 110
# (aws-setup tracker #30).
reserved_concurrency       = -1
log_retention_days         = 30
enable_pitr                = true
enable_deletion_protection = true
domain_names               = ["craigdevjohnson.com", "www.craigdevjohnson.com"]
request_custom_domain      = true
activate_custom_domain     = true

# LPS history sync (#80). Gates 3 and 4 of
# docs/deployment/2026-09-26-lps-history-activation-readiness.md accepted these
# limits and the daily time on September 30, 2026. Every switch stays off until
# the packet's remaining gates close; turning one on is a separate reviewed
# change, planned and applied by Craig, never passed with -var.
enable_soccer_history              = false
activate_soccer_history_collection = false
activate_soccer_history_schedule   = false
soccer_history_schedule_expression = "cron(30 10 * * ? *)" # 10:30 UTC, 03:30 Pacific daylight time
soccer_history_limits = {
  max_enrolled_teams      = 40
  reserved_player_slots   = 30
  max_requests_per_run    = 120
  max_retries_per_team    = 1
  min_request_interval_ms = 1000
  worker_timeout_seconds  = 300
}

# Production site identity (#88), from task cognito-site-prod-export on 2026-10-03.
# Production grants only soccer; this root refuses a management grant.
site = {
  cognito_domain       = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
  cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_6oBD4npvf"
  cognito_client_id    = "4h3jrtlq245orultjdltouq02v"
  redirect_uri         = "https://craigdevjohnson.com/auth/callback"
  logout_uri           = "https://craigdevjohnson.com/sign-in"
  allow_local_callback = false
  invitations          = { "craigdevjohnson@gmail.com" = ["soccer"] }
}
