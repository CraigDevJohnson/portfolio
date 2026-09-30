locals {
  function_name        = var.name_prefix
  google_table         = "${var.name_prefix}-google-connections"
  soccer_table         = "${var.name_prefix}-soccer-sessions"
  soccer_archive_table = "${var.name_prefix}-soccer-history"
  ssm_base             = "/portfolio/lambda/${var.environment}"
  ssm_names            = toset(concat(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"], var.site == null ? [] : ["SITE_SESSION_KEY"]))
  ssm_paths            = { for name in local.ssm_names : name => "${local.ssm_base}/${name}" }
  image_uri            = "${var.ecr_repository_url}@${var.image_digest}"
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

  # The portal follows site sign-in and the management grant; of the former
  # management settings, the application still reads only its AWS region.
  management_environment = var.management == null ? {} : {
    MGMT_AWS_REGION = var.aws_region
  }
}
