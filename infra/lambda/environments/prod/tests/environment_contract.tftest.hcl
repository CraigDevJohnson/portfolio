mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = { account_id = "111122223333" }
  }

  mock_data "aws_partition" {
    defaults = { partition = "aws" }
  }

  mock_data "aws_kms_alias" {
    defaults = { target_key_arn = "arn:aws:kms:us-west-2:111122223333:key/00000000-0000-0000-0000-000000000000" }
  }

  mock_data "aws_iam_policy_document" {
    defaults = { json = "{}" }
  }

  mock_resource "aws_acm_certificate" {
    defaults = {
      arn = "arn:aws:acm:us-west-2:111122223333:certificate/00000000-0000-0000-0000-000000000000"
      domain_validation_options = [
        { domain_name = "craigdevjohnson.com", resource_record_name = "_apex.craigdevjohnson.com", resource_record_type = "CNAME", resource_record_value = "_apex.acm-validations.aws" },
        { domain_name = "www.craigdevjohnson.com", resource_record_name = "_www.www.craigdevjohnson.com", resource_record_type = "CNAME", resource_record_value = "_www.acm-validations.aws" },
      ]
    }
  }

  mock_resource "aws_apigatewayv2_domain_name" {
    defaults = {
      domain_name_configuration = {
        target_domain_name = "example.execute-api.us-west-2.amazonaws.com"
        hosted_zone_id     = "example-zone"
      }
    }
  }

  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-west-2:111122223333:log-group:portfolio-test" }
  }

  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = { arn = "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test" }
  }

  mock_resource "aws_dynamodb_table" {
    defaults = { arn = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test" }
  }

  mock_resource "aws_sqs_queue" {
    defaults = { arn = "arn:aws:sqs:us-west-2:111122223333:portfolio-lambda-prod-soccer-history-failures" }
  }

  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::111122223333:role/portfolio-lambda-test" }
  }

  mock_resource "aws_apigatewayv2_api" {
    defaults = {
      api_endpoint  = "https://test.execute-api.us-west-2.amazonaws.com"
      execution_arn = "arn:aws:execute-api:us-west-2:111122223333:test-api"
      id            = "test-api"
    }
  }

  mock_resource "aws_lambda_function" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod/invocations"
      version    = "1"
    }
  }

  mock_resource "aws_lambda_alias" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod:live"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod:live/invocations"
    }
  }
}

variables {
  aws_account_id    = "111122223333"
  alarm_action_arns = ["arn:aws:sns:us-west-2:111122223333:alerts"]
}

run "production_environment_contract" {
  command = plan

  assert {
    condition = (
      var.environment == "prod" &&
      var.name_prefix == "portfolio-lambda-prod" &&
      var.aws_region == "us-west-2" &&
      var.ecr_repository_url == "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases" &&
      var.image_digest == "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      var.lambda_memory_mb == 512 &&
      var.lambda_timeout_seconds == 29 &&
      var.reserved_concurrency == -1 && # temporary until the Lambda quota is raised; see prod.auto.tfvars
      var.log_retention_days == 30 &&
      var.enable_pitr &&
      var.enable_deletion_protection &&
      tolist(var.alarm_action_arns) == tolist(["arn:aws:sns:us-west-2:111122223333:alerts"]) &&
      toset(var.domain_names) == toset(["craigdevjohnson.com", "www.craigdevjohnson.com"]) &&
      var.request_custom_domain &&
      var.activate_custom_domain
    )
    error_message = "production must use the reviewed isolated environment values"
  }

  assert {
    condition = (
      output.environment == "prod" &&
      output.image_uri == "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      output.lambda_function_name == "portfolio-lambda-prod" &&
      output.lambda_function_arn == "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod" &&
      output.lambda_published_version == "1" &&
      output.lambda_alias_name == "live" &&
      output.lambda_alias_arn == "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod:live" &&
      output.api_id == "test-api" &&
      output.api_default_url == "https://test.execute-api.us-west-2.amazonaws.com"
    )
    error_message = "production string outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.lambda_execution_role_name == "portfolio-lambda-prod-execution" &&
      output.lambda_execution_permissions_boundary_arn == "arn:aws:iam::111122223333:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
      output.lambda_runtime_policy_name == "portfolio-lambda-prod-runtime" &&
      output.api_name == "portfolio-lambda-prod-http" &&
      output.alarm_names == tolist([
        "portfolio-lambda-prod-api-5xx",
        "portfolio-lambda-prod-api-latency",
        "portfolio-lambda-prod-lambda-duration",
        "portfolio-lambda-prod-lambda-errors",
        "portfolio-lambda-prod-lambda-throttles",
      ])
    )
    error_message = "production deployment names and execution boundary must remain deterministic"
  }

  assert {
    condition = (
      output.lambda_log_group_name == "/aws/lambda/portfolio-lambda-prod" &&
      output.api_access_log_group_name == "/aws/apigateway/portfolio-lambda-prod/access" &&
      output.google_connection_table_name == "portfolio-lambda-prod-google-connections" &&
      output.google_connection_table_arn == "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test" &&
      output.soccer_session_table_name == "portfolio-lambda-prod-soccer-sessions" &&
      output.soccer_session_table_arn == "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test"
    )
    error_message = "production storage and log outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.soccer_history_table_name == null &&
      output.soccer_history_table_arn == null &&
      output.soccer_history_worker_function_name == null &&
      output.soccer_history_schedule_name == null
    )
    error_message = "production must not plan the durable Soccer history table, worker or schedule before its activation review"
  }

  # Gates 3 and 4 of the LPS history readiness packet accepted these limits and
  # the 10:30 UTC daily run on September 30, 2026. Production stays off until
  # every remaining gate closes.
  assert {
    condition = (
      !var.enable_soccer_history &&
      !var.activate_soccer_history_collection &&
      !var.activate_soccer_history_schedule &&
      var.soccer_history_schedule_expression == "cron(30 10 * * ? *)" &&
      var.soccer_history_limits == {
        max_enrolled_teams      = 40
        reserved_player_slots   = 30
        max_requests_per_run    = 120
        max_retries_per_team    = 1
        min_request_interval_ms = 1000
        worker_timeout_seconds  = 300
      }
    )
    error_message = "production must carry the accepted history limits and schedule with every history switch off"
  }

  assert {
    condition = output.ssm_parameter_paths == tomap({
      CLIENT_ID_KEY     = "/portfolio/lambda/prod/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY = "/portfolio/lambda/prod/CLIENT_SECRET_KEY"
      LPS_SESSION_KEY   = "/portfolio/lambda/prod/LPS_SESSION_KEY"
      SITE_SESSION_KEY  = "/portfolio/lambda/prod/SITE_SESSION_KEY"
    })
    error_message = "production must expose only its four SSM paths, including SITE_SESSION_KEY now that its reviewed site identity is set"
  }

  assert {
    condition = (
      output.alarm_arns == tolist([
        "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test",
      ]) &&
      output.certificate_arn == "arn:aws:acm:us-west-2:111122223333:certificate/00000000-0000-0000-0000-000000000000" &&
      length(output.acm_validation_records) == 2 &&
      output.api_gateway_domain_targets == tomap({
        "craigdevjohnson.com"     = "example.execute-api.us-west-2.amazonaws.com"
        "www.craigdevjohnson.com" = "example.execute-api.us-west-2.amazonaws.com"
      }) &&
      output.oauth_redirect_uris == tolist([
        "https://craigdevjohnson.com/soccer",
        "https://www.craigdevjohnson.com/soccer",
      ])
    )
    error_message = "production collection and nullable outputs must keep their reviewed values and shapes"
  }
}

run "reject_other_alarm_topic" {
  command = plan

  variables {
    alarm_action_arns = ["arn:aws:sns:us-west-2:111122223333:portfolio-alerts"]
  }

  expect_failures = [var.alarm_action_arns]
}

run "production_site_runtime_contract" {
  command = plan

  variables {
    site = {
      cognito_domain       = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_ProdSite"
      cognito_client_id    = "prodsiteclient"
      redirect_uri         = "https://craigdevjohnson.com/auth/callback"
      logout_uri           = "https://craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer"] }
      allow_local_callback = false
    }
  }

  assert {
    condition     = output.ssm_parameter_paths.SITE_SESSION_KEY == "/portfolio/lambda/prod/SITE_SESSION_KEY"
    error_message = "production must forward its site identity with a production-only session parameter path"
  }
}

# Proves the root hands every history input from prod.auto.tfvars to the
# service module: switching them on here plans each stage. This is not a
# reviewed production configuration.
run "history_inputs_reach_the_service" {
  command = plan

  variables {
    enable_soccer_history              = true
    activate_soccer_history_collection = true
    activate_soccer_history_schedule   = true
  }

  assert {
    condition = (
      output.soccer_history_table_name == "portfolio-lambda-prod-soccer-history" &&
      output.soccer_history_worker_function_name == "portfolio-lambda-prod-soccer-history" &&
      output.soccer_history_schedule_name == "portfolio-lambda-prod-soccer-history-daily" &&
      output.alarm_names == tolist([
        "portfolio-lambda-prod-api-5xx",
        "portfolio-lambda-prod-api-latency",
        "portfolio-lambda-prod-lambda-duration",
        "portfolio-lambda-prod-lambda-errors",
        "portfolio-lambda-prod-lambda-throttles",
        "portfolio-lambda-prod-soccer-history-admission-rejected",
        "portfolio-lambda-prod-soccer-history-dead-letter",
        "portfolio-lambda-prod-soccer-history-errors",
        "portfolio-lambda-prod-soccer-history-incomplete",
      ])
    )
    error_message = "production must pass the history switches, limits and schedule to the service module"
  }
}

# Decision 6 (2026-09-30): production invites Craig with soccer only. Its
# Lambda role has no EC2 or metric grants, so a management grant would only
# open a portal that fails.
run "production_site_rejects_management_grant" {
  command = plan

  variables {
    site = {
      cognito_domain       = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_ProdSite"
      cognito_client_id    = "prodsiteclient"
      redirect_uri         = "https://craigdevjohnson.com/auth/callback"
      logout_uri           = "https://craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer"], "second@example.com" = ["management"] }
      allow_local_callback = false
    }
  }

  expect_failures = [var.site]
}
