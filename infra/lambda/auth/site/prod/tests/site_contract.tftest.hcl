mock_provider "aws" {
  mock_resource "aws_cognito_user_pool" {
    defaults = {
      id       = "us-west-2_mockprodsite"
      endpoint = "cognito-idp.us-west-2.amazonaws.com/us-west-2_mockprodsite"
    }
  }

  mock_resource "aws_cognito_user_pool_client" {
    defaults = { id = "mockprodsiteclient" }
  }
}

variables {
  aws_account_id       = "111122223333"
  google_client_id     = "mock-prod-google-client.apps.googleusercontent.com"
  google_client_secret = "mock-prod-google-secret"
}

run "production_runtime_handoff" {
  command = plan

  assert {
    condition = (
      output.cognito_user_pool_id == "us-west-2_mockprodsite" &&
      output.google_redirect_uri == "https://portfolio-lambda-prod-site-111122223333.auth.us-west-2.amazoncognito.com/oauth2/idpresponse" &&
      output.session_parameter_path == "/portfolio/lambda/prod/SITE_SESSION_KEY" &&
      output.site_runtime.cognito_domain == "https://portfolio-lambda-prod-site-111122223333.auth.us-west-2.amazoncognito.com" &&
      output.site_runtime.cognito_issuer == "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_mockprodsite" &&
      output.site_runtime.cognito_client_id == "mockprodsiteclient" &&
      output.site_runtime.redirect_uri == "https://craigdevjohnson.com/auth/callback" &&
      output.site_runtime.logout_uri == "https://craigdevjohnson.com/sign-in" &&
      length(output.site_runtime.invitations) == 1 &&
      toset(output.site_runtime.invitations["craigdevjohnson@gmail.com"]) == toset(["soccer", "management"]) &&
      !output.site_runtime.allow_local_callback &&
      !strcontains(jsonencode(output.site_runtime), "mock-prod-google-secret")
    )
    error_message = "production runtime output must contain only its reviewed site identity and grants"
  }
}

run "accept_reviewed_domain_prefix_override" {
  command = plan

  variables {
    cognito_domain_prefix = "portfolio-lambda-prod-site-reviewed"
  }

  assert {
    condition     = output.site_runtime.cognito_domain == "https://portfolio-lambda-prod-site-reviewed.auth.us-west-2.amazoncognito.com"
    error_message = "a reviewed override that keeps the environment prefix must reach site_runtime"
  }
}

run "reject_domain_prefix_outside_the_environment" {
  command = plan

  variables {
    cognito_domain_prefix = "craig-site-auth"
  }

  expect_failures = [var.cognito_domain_prefix]
}

run "reject_other_environment_domain_prefix" {
  command = plan

  variables {
    cognito_domain_prefix = "portfolio-lambda-dev-site-reviewed"
  }

  expect_failures = [var.cognito_domain_prefix]
}

run "reject_overlong_domain_prefix" {
  command = plan

  variables {
    cognito_domain_prefix = "portfolio-lambda-prod-site-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }

  expect_failures = [var.cognito_domain_prefix]
}
