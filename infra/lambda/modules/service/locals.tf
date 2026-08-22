locals {
  function_name = var.name_prefix
  google_table  = "${var.name_prefix}-google-connections"
  soccer_table  = "${var.name_prefix}-soccer-sessions"
  ssm_base      = "/portfolio/lambda/${var.environment}"
  ssm_names     = toset(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"])
  ssm_paths     = { for name in local.ssm_names : name => "${local.ssm_base}/${name}" }
  image_uri     = "${var.ecr_repository_url}@${var.image_digest}"
}
