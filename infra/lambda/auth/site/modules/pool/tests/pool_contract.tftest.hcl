mock_provider "aws" {
  mock_resource "aws_cognito_user_pool" {
    defaults = {
      id       = "us-west-2_mocksite"
      endpoint = "cognito-idp.us-west-2.amazonaws.com/us-west-2_mocksite"
    }
  }

  mock_resource "aws_cognito_user_pool_client" {
    defaults = { id = "mocksiteclient" }
  }
}

variables {
  environment           = "dev"
  cognito_domain_prefix = "portfolio-lambda-dev-site-111122223333"
  google_client_id      = "mock-dev-google-client.apps.googleusercontent.com"
  google_client_secret  = "mock-dev-google-secret"
}

run "development_site_pool_contract" {
  command = plan

  assert {
    condition = (
      aws_cognito_user_pool.site.name == "portfolio-lambda-dev-site" &&
      aws_cognito_user_pool.site.user_pool_tier == "ESSENTIALS" &&
      aws_cognito_user_pool.site.admin_create_user_config[0].allow_admin_create_user_only &&
      !aws_cognito_user_pool.site.username_configuration[0].case_sensitive &&
      aws_cognito_identity_provider.google.provider_name == "Google" &&
      aws_cognito_identity_provider.google.provider_type == "Google" &&
      aws_cognito_identity_provider.google.provider_details.authorize_scopes == "openid email profile" &&
      aws_cognito_identity_provider.google.attribute_mapping == tomap({
        email          = "email"
        email_verified = "email_verified"
        name           = "name"
        username       = "sub"
      })
    )
    error_message = "development site pool must be invite-only and Google-federated with verified email"
  }

  # Cognito adds these Google settings after create; declaring them keeps
  # every later plan a no-op instead of an update that Cognito undoes.
  assert {
    condition = (
      aws_cognito_identity_provider.google.provider_details.attributes_url == "https://people.googleapis.com/v1/people/me?personFields=" &&
      aws_cognito_identity_provider.google.provider_details.attributes_url_add_attributes == "true" &&
      aws_cognito_identity_provider.google.provider_details.authorize_url == "https://accounts.google.com/o/oauth2/v2/auth" &&
      aws_cognito_identity_provider.google.provider_details.oidc_issuer == "https://accounts.google.com" &&
      aws_cognito_identity_provider.google.provider_details.token_request_method == "POST" &&
      aws_cognito_identity_provider.google.provider_details.token_url == "https://www.googleapis.com/oauth2/v4/token"
    )
    error_message = "the Google provider must declare the settings Cognito adds, so plans converge"
  }

  assert {
    condition = (
      aws_cognito_user_pool_client.site.name == "portfolio-lambda-dev-site-web" &&
      !aws_cognito_user_pool_client.site.generate_secret &&
      aws_cognito_user_pool_client.site.allowed_oauth_flows_user_pool_client &&
      toset(aws_cognito_user_pool_client.site.allowed_oauth_flows) == toset(["code"]) &&
      toset(aws_cognito_user_pool_client.site.allowed_oauth_scopes) == toset(["openid", "email", "profile"]) &&
      toset(aws_cognito_user_pool_client.site.supported_identity_providers) == toset(["Google"]) &&
      toset(aws_cognito_user_pool_client.site.explicit_auth_flows) == toset(["ALLOW_REFRESH_TOKEN_AUTH"]) &&
      toset(aws_cognito_user_pool_client.site.callback_urls) == toset(["https://dev.craigdevjohnson.com/auth/callback"]) &&
      toset(aws_cognito_user_pool_client.site.logout_urls) == toset(["https://dev.craigdevjohnson.com/sign-in"])
    )
    error_message = "development app client must use site callbacks and Google authorization code only"
  }

  assert {
    condition = (
      aws_cognito_user_pool_domain.site.domain == "portfolio-lambda-dev-site-111122223333" &&
      aws_cognito_user_pool_domain.site.managed_login_version == 2 &&
      aws_cognito_managed_login_branding.site.use_cognito_provided_values &&
      output.cognito_issuer == "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_mocksite" &&
      output.cognito_client_id == "mocksiteclient" &&
      output.google_redirect_uri == "https://portfolio-lambda-dev-site-111122223333.auth.us-west-2.amazoncognito.com/oauth2/idpresponse"
    )
    error_message = "development domain and output must match the independent site identity"
  }
}

run "production_site_pool_contract" {
  command = plan

  variables {
    environment           = "prod"
    cognito_domain_prefix = "portfolio-lambda-prod-site-111122223333"
  }

  assert {
    condition = (
      aws_cognito_user_pool.site.name == "portfolio-lambda-prod-site" &&
      aws_cognito_user_pool_client.site.name == "portfolio-lambda-prod-site-web" &&
      toset(aws_cognito_user_pool_client.site.callback_urls) == toset(["https://craigdevjohnson.com/auth/callback"]) &&
      toset(aws_cognito_user_pool_client.site.logout_urls) == toset(["https://craigdevjohnson.com/sign-in"]) &&
      aws_cognito_user_pool_domain.site.domain == "portfolio-lambda-prod-site-111122223333" &&
      output.cognito_domain == "https://portfolio-lambda-prod-site-111122223333.auth.us-west-2.amazoncognito.com"
    )
    error_message = "production pool, client, domain, and callbacks must be distinct from development"
  }
}

run "production_rejects_loopback_callback" {
  command = plan

  variables {
    environment           = "prod"
    cognito_domain_prefix = "portfolio-lambda-prod-site-111122223333"
    enable_local_callback = true
  }

  expect_failures = [var.enable_local_callback]
}

run "development_loopback_requires_opt_in" {
  command = plan

  variables {
    enable_local_callback = true
  }

  assert {
    condition = toset(aws_cognito_user_pool_client.site.callback_urls) == toset([
      "https://dev.craigdevjohnson.com/auth/callback",
      "http://localhost:8080/auth/callback",
    ])
    error_message = "development loopback callback must require explicit opt-in"
  }
}
