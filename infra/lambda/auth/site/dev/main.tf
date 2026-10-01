locals {
  session_parameter_path = "/portfolio/lambda/dev/SITE_SESSION_KEY"
  cognito_domain_prefix  = coalesce(var.cognito_domain_prefix, "portfolio-lambda-dev-site-${var.aws_account_id}")
}

module "site" {
  source = "../modules/pool"

  environment           = "dev"
  google_client_id      = var.google_client_id
  google_client_secret  = var.google_client_secret
  cognito_domain_prefix = local.cognito_domain_prefix
  enable_local_callback = var.enable_local_callback
}
