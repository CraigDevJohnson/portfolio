locals {
  aws_region          = "us-west-2"
  name_prefix         = "portfolio-lambda-${var.environment}-site"
  site_origin         = var.environment == "dev" ? "https://dev.craigdevjohnson.com" : "https://craigdevjohnson.com"
  callback_uri        = "${local.site_origin}/auth/callback"
  logout_uri          = "${local.site_origin}/sign-in"
  local_callback_uri  = "http://localhost:8080/auth/callback"
  cognito_domain      = "https://${var.cognito_domain_prefix}.auth.${local.aws_region}.amazoncognito.com"
  google_redirect_uri = "${local.cognito_domain}/oauth2/idpresponse"
}

resource "aws_cognito_user_pool" "site" {
  name           = local.name_prefix
  user_pool_tier = "ESSENTIALS"

  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  username_configuration {
    case_sensitive = false
  }
}

resource "aws_cognito_identity_provider" "google" {
  user_pool_id  = aws_cognito_user_pool.site.id
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

resource "aws_cognito_user_pool_client" "site" {
  name         = "${local.name_prefix}-web"
  user_pool_id = aws_cognito_user_pool.site.id

  generate_secret                      = false
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  supported_identity_providers         = [aws_cognito_identity_provider.google.provider_name]
  explicit_auth_flows                  = ["ALLOW_REFRESH_TOKEN_AUTH"]
  callback_urls                        = concat([local.callback_uri], var.enable_local_callback ? [local.local_callback_uri] : [])
  logout_urls                          = [local.logout_uri]
  read_attributes                      = ["email", "email_verified", "name"]
  write_attributes                     = ["email", "name"]
}

resource "aws_cognito_user_pool_domain" "site" {
  domain                = var.cognito_domain_prefix
  user_pool_id          = aws_cognito_user_pool.site.id
  managed_login_version = 2
}

resource "aws_cognito_managed_login_branding" "site" {
  user_pool_id                = aws_cognito_user_pool.site.id
  client_id                   = aws_cognito_user_pool_client.site.id
  use_cognito_provided_values = true

  depends_on = [aws_cognito_user_pool_domain.site]
}
