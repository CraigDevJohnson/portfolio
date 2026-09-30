mock_provider "aws" {
  mock_resource "aws_cognito_user_pool" {
    defaults = {
      id       = "us-west-2_mockdevsite"
      endpoint = "cognito-idp.us-west-2.amazonaws.com/us-west-2_mockdevsite"
    }
  }

  mock_resource "aws_cognito_user_pool_client" {
    defaults = { id = "mockdevsiteclient" }
  }
}

variables {
  aws_account_id       = "111122223333"
  google_client_id     = "mock-dev-google-client.apps.googleusercontent.com"
  google_client_secret = "mock-dev-google-secret"
}

run "development_runtime_handoff" {
  command = plan

  assert {
    condition = (
      output.cognito_user_pool_id == "us-west-2_mockdevsite" &&
      output.google_redirect_uri == "https://portfolio-lambda-dev-site-111122223333.auth.us-west-2.amazoncognito.com/oauth2/idpresponse" &&
      output.session_parameter_path == "/portfolio/lambda/dev/SITE_SESSION_KEY" &&
      output.site_runtime.cognito_domain == "https://portfolio-lambda-dev-site-111122223333.auth.us-west-2.amazoncognito.com" &&
      output.site_runtime.cognito_issuer == "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_mockdevsite" &&
      output.site_runtime.cognito_client_id == "mockdevsiteclient" &&
      output.site_runtime.redirect_uri == "https://dev.craigdevjohnson.com/auth/callback" &&
      output.site_runtime.logout_uri == "https://dev.craigdevjohnson.com/sign-in" &&
      length(output.site_runtime.invitations) == 1 &&
      toset(output.site_runtime.invitations["craigdevjohnson@gmail.com"]) == toset(["soccer", "management"]) &&
      !output.site_runtime.allow_local_callback &&
      !strcontains(jsonencode(output.site_runtime), "mock-dev-google-secret")
    )
    error_message = "development runtime output must contain only its reviewed site identity and grants"
  }
}

run "accept_reviewed_domain_prefix_override" {
  command = plan

  variables {
    cognito_domain_prefix = "portfolio-lambda-dev-site-reviewed"
  }

  assert {
    condition     = output.site_runtime.cognito_domain == "https://portfolio-lambda-dev-site-reviewed.auth.us-west-2.amazoncognito.com"
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
    cognito_domain_prefix = "portfolio-lambda-prod-site-reviewed"
  }

  expect_failures = [var.cognito_domain_prefix]
}

run "reject_overlong_domain_prefix" {
  command = plan

  variables {
    cognito_domain_prefix = "portfolio-lambda-dev-site-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }

  expect_failures = [var.cognito_domain_prefix]
}
