locals {
  session_parameter_path = "/portfolio/lambda/prod/SITE_SESSION_KEY"
  cognito_domain_prefix  = coalesce(var.cognito_domain_prefix, "portfolio-lambda-prod-site-${var.aws_account_id}")
}

module "site" {
  source = "../modules/pool"

  environment           = "prod"
  google_client_id      = var.google_client_id
  google_client_secret  = var.google_client_secret
  cognito_domain_prefix = local.cognito_domain_prefix
}
