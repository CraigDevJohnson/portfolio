locals {
  aws_account_id           = "180294223248"
  aws_region               = "us-west-2"
  user_pool_name           = "portfolio-lambda-dev-mgmt"
  app_client_name          = "portfolio-lambda-dev-mgmt-web"
  callback_uri             = "https://dev.craigdevjohnson.com/callback"
  local_callback_uri       = "http://localhost:8080/callback"
  logout_uri               = "https://dev.craigdevjohnson.com/login"
  session_parameter_path   = "/portfolio/lambda/dev/MGMT_SESSION_KEY"
  allowed_emails           = toset(["craigdevjohnson@gmail.com"])
  ec2_management_tag_key   = "PortfolioManagement"
  ec2_management_tag_value = "dev"
  cognito_domain           = "https://${var.cognito_domain_prefix}.auth.${local.aws_region}.amazoncognito.com"
  cognito_issuer           = "https://${aws_cognito_user_pool.management.endpoint}"
  google_redirect_uri      = "${local.cognito_domain}/oauth2/idpresponse"
}

resource "aws_cognito_user_pool" "management" {
  name           = local.user_pool_name
  user_pool_tier = "ESSENTIALS"

  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  username_configuration {
    case_sensitive = false
  }
}

resource "aws_cognito_identity_provider" "google" {
  user_pool_id  = aws_cognito_user_pool.management.id
  provider_name = "Google"
  provider_type = "Google"

  provider_details = {
    authorize_scopes = "openid email profile"
    client_id        = var.google_client_id
    client_secret    = var.google_client_secret
  }

  attribute_mapping = {
    email          = "email"
    email_verified = "email_verified"
    name           = "name"
  }
}

resource "aws_cognito_user_pool_client" "management" {
  name         = local.app_client_name
  user_pool_id = aws_cognito_user_pool.management.id

  generate_secret                      = false
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  supported_identity_providers         = [aws_cognito_identity_provider.google.provider_name]
  explicit_auth_flows                  = ["ALLOW_REFRESH_TOKEN_AUTH"]
  callback_urls                        = concat([local.callback_uri], var.enable_local_callback ? [local.local_callback_uri] : [])
  logout_urls                          = [local.logout_uri]
  read_attributes                      = ["email", "email_verified", "name"]
  # Cognito forbids client writes to email_verified; Google supplies its value
  # through the IdP mapping above. See the PR #71 review record for AWS guidance.
  write_attributes = ["email", "name"]
}

resource "aws_cognito_user_pool_domain" "management" {
  domain                = var.cognito_domain_prefix
  user_pool_id          = aws_cognito_user_pool.management.id
  managed_login_version = 2
}

resource "aws_cognito_managed_login_branding" "management" {
  user_pool_id                = aws_cognito_user_pool.management.id
  client_id                   = aws_cognito_user_pool_client.management.id
  use_cognito_provided_values = true

  depends_on = [aws_cognito_user_pool_domain.management]
}
