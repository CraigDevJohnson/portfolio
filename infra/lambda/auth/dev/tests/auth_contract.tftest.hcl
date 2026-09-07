mock_provider "aws" {
  mock_resource "aws_cognito_user_pool" {
    defaults = {
      id       = "us-west-2_mockpool"
      endpoint = "cognito-idp.us-west-2.amazonaws.com/us-west-2_mockpool"
    }
  }

  mock_resource "aws_cognito_user_pool_client" {
    defaults = { id = "mock-public-client-id" }
  }
}

variables {
  google_client_id     = "mock-google-client-id.apps.googleusercontent.com"
  google_client_secret = "mock-google-client-secret-sentinel"
}

run "default_google_only_auth_contract" {
  command = plan

  assert {
    condition = (
      local.aws_account_id == "180294223248" &&
      local.aws_region == "us-west-2" &&
      local.user_pool_name == "portfolio-lambda-dev-mgmt" &&
      local.app_client_name == "portfolio-lambda-dev-mgmt-web" &&
      local.allowed_emails == toset(["craigdevjohnson@gmail.com"]) &&
      local.ec2_management_tag_key == "PortfolioManagement" &&
      local.ec2_management_tag_value == "dev"
    )
    error_message = "development identity, names, allowlist, and management tag must remain deterministic"
  }

  assert {
    condition = (
      aws_cognito_user_pool.management.name == "portfolio-lambda-dev-mgmt" &&
      aws_cognito_user_pool.management.user_pool_tier == "ESSENTIALS" &&
      aws_cognito_user_pool.management.admin_create_user_config[0].allow_admin_create_user_only &&
      !aws_cognito_user_pool.management.username_configuration[0].case_sensitive
    )
    error_message = "the development user pool must remain administrator-created and case-insensitive"
  }

  assert {
    condition = (
      aws_cognito_identity_provider.google.provider_name == "Google" &&
      aws_cognito_identity_provider.google.provider_type == "Google" &&
      aws_cognito_identity_provider.google.provider_details.authorize_scopes == "openid email profile" &&
      aws_cognito_identity_provider.google.provider_details.client_id == var.google_client_id &&
      aws_cognito_identity_provider.google.provider_details.client_secret == var.google_client_secret &&
      aws_cognito_identity_provider.google.attribute_mapping == tomap({
        email          = "email"
        email_verified = "email_verified"
        name           = "name"
      })
    )
    error_message = "Google must be the configured provider with the reviewed scopes and verified-email mapping"
  }

  assert {
    condition = (
      !aws_cognito_user_pool_client.management.generate_secret &&
      aws_cognito_user_pool_client.management.allowed_oauth_flows_user_pool_client &&
      toset(aws_cognito_user_pool_client.management.allowed_oauth_flows) == toset(["code"]) &&
      toset(aws_cognito_user_pool_client.management.allowed_oauth_scopes) == toset(["openid", "email", "profile"]) &&
      toset(aws_cognito_user_pool_client.management.supported_identity_providers) == toset(["Google"]) &&
      toset(aws_cognito_user_pool_client.management.explicit_auth_flows) == toset(["ALLOW_REFRESH_TOKEN_AUTH"]) &&
      toset(aws_cognito_user_pool_client.management.read_attributes) == toset(["email", "email_verified", "name"]) &&
      toset(aws_cognito_user_pool_client.management.write_attributes) == toset(["email", "name"])
    )
    error_message = "the app client must remain public, Google-only, code-flow-only, and free of password, SRP, admin, or custom authentication"
  }

  assert {
    condition = (
      aws_cognito_user_pool_client.management.callback_urls == toset(["https://dev.craigdevjohnson.com/callback"]) &&
      aws_cognito_user_pool_client.management.logout_urls == toset(["https://dev.craigdevjohnson.com/login"]) &&
      !contains(aws_cognito_user_pool_client.management.callback_urls, "http://localhost:8080/callback")
    )
    error_message = "the default client URLs must use only the approved development HTTPS callback and logout"
  }

  assert {
    condition = (
      aws_cognito_user_pool_domain.management.domain == "portfolio-lambda-dev-mgmt-180294223248" &&
      aws_cognito_user_pool_domain.management.managed_login_version == 2 &&
      aws_cognito_managed_login_branding.management.use_cognito_provided_values
    )
    error_message = "the managed-login domain and branding must use the reviewed defaults and version 2"
  }

  assert {
    condition = (
      output.cognito_user_pool_id == "us-west-2_mockpool" &&
      output.cognito_domain == "https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com" &&
      output.cognito_issuer == "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_mockpool" &&
      output.cognito_client_id == "mock-public-client-id" &&
      output.google_redirect_uri == "https://portfolio-lambda-dev-mgmt-180294223248.auth.us-west-2.amazoncognito.com/oauth2/idpresponse" &&
      output.session_parameter_path == "/portfolio/lambda/dev/MGMT_SESSION_KEY"
    )
    error_message = "individual outputs must expose only the reviewed public Cognito settings and session parameter name"
  }

  assert {
    condition = (
      output.management_runtime.cognito_domain == output.cognito_domain &&
      output.management_runtime.cognito_issuer == output.cognito_issuer &&
      output.management_runtime.cognito_client_id == output.cognito_client_id &&
      output.management_runtime.redirect_uri == "https://dev.craigdevjohnson.com/callback" &&
      output.management_runtime.logout_uri == "https://dev.craigdevjohnson.com/login" &&
      output.management_runtime.allowed_emails == toset(["craigdevjohnson@gmail.com"]) &&
      !output.management_runtime.allow_local_callback &&
      output.management_runtime.ec2_management_tag_key == "PortfolioManagement" &&
      output.management_runtime.ec2_management_tag_value == "dev" &&
      !strcontains(jsonencode(output.management_runtime), "mock-google-client-secret-sentinel")
    )
    error_message = "management_runtime must match the public Task 4 handoff contract and exclude provider credentials"
  }

  assert {
    condition = !strcontains(jsonencode({
      cognito_user_pool_id   = output.cognito_user_pool_id
      cognito_domain         = output.cognito_domain
      cognito_issuer         = output.cognito_issuer
      cognito_client_id      = output.cognito_client_id
      google_redirect_uri    = output.google_redirect_uri
      session_parameter_path = output.session_parameter_path
      management_runtime     = output.management_runtime
    }), "mock-google-client-secret-sentinel")
    error_message = "provider credentials must be absent from every declared output"
  }
}

run "explicit_local_callback_opt_in" {
  command = plan

  variables {
    enable_local_callback = true
  }

  assert {
    condition = (
      toset(aws_cognito_user_pool_client.management.callback_urls) == toset([
        "https://dev.craigdevjohnson.com/callback",
        "http://localhost:8080/callback",
      ]) &&
      output.management_runtime.allow_local_callback
    )
    error_message = "the loopback callback must appear only after explicit local opt-in"
  }
}

run "reject_empty_google_client_id" {
  command = plan

  variables {
    google_client_id = " "
  }

  expect_failures = [var.google_client_id]
}

run "reject_empty_google_client_secret" {
  command = plan

  variables {
    google_client_secret = " "
  }

  expect_failures = [var.google_client_secret]
}

run "reject_invalid_domain_prefix" {
  command = plan

  variables {
    cognito_domain_prefix = "Invalid Domain"
  }

  expect_failures = [var.cognito_domain_prefix]
}
