locals {
  function_name = var.name_prefix
  google_table  = "${var.name_prefix}-google-connections"
  soccer_table  = "${var.name_prefix}-soccer-sessions"
  ssm_base      = "/portfolio/lambda/${var.environment}"
  ssm_names     = toset(concat(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"], var.management == null ? [] : ["MGMT_SESSION_KEY"]))
  ssm_paths     = { for name in local.ssm_names : name => "${local.ssm_base}/${name}" }
  image_uri     = "${var.ecr_repository_url}@${var.image_digest}"
}

locals {
  management_environment = var.management == null ? {} : {
    MGMT_SESSION_KEY          = local.ssm_paths.MGMT_SESSION_KEY
    MGMT_COGNITO_DOMAIN       = var.management.cognito_domain
    MGMT_COGNITO_ISSUER       = var.management.cognito_issuer
    MGMT_COGNITO_CLIENT_ID    = var.management.cognito_client_id
    MGMT_COGNITO_REDIRECT_URI = var.management.redirect_uri
    MGMT_COGNITO_LOGOUT_URI   = var.management.logout_uri
    MGMT_ALLOWED_EMAILS       = join(",", sort(tolist(var.management.allowed_emails)))
    MGMT_ALLOW_LOCAL_CALLBACK = tostring(var.management.allow_local_callback)
    MGMT_AWS_REGION           = var.aws_region
  }
}
