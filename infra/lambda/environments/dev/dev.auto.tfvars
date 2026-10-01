environment                = "dev"
name_prefix                = "portfolio-lambda-dev"
aws_region                 = "us-west-2"
lambda_memory_mb           = 512
lambda_timeout_seconds     = 29
reserved_concurrency       = -1
log_retention_days         = 14
enable_pitr                = false
enable_deletion_protection = false
alarm_action_arns          = []
domain_names               = ["dev.craigdevjohnson.com"]
request_custom_domain      = true
activate_custom_domain     = true

# LPS history sync (#80). Gates 3 and 4 of
# docs/deployment/2026-09-26-lps-history-activation-readiness.md accepted these
# limits on September 30, 2026. Craig decided the same day that development
# neither collects nor runs the daily schedule: collection would need its own
# alert destination (a nonempty alarm_action_arns) and the schedule would
# double LPS traffic. Every switch stays off and development has no schedule.
enable_soccer_history              = false
activate_soccer_history_collection = false
activate_soccer_history_schedule   = false
soccer_history_schedule_expression = null
soccer_history_limits = {
  max_enrolled_teams      = 40
  reserved_player_slots   = 30
  max_requests_per_run    = 120
  max_retries_per_team    = 1
  min_request_interval_ms = 1000
  worker_timeout_seconds  = 300
}
