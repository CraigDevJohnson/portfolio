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

  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-west-2:111122223333:log-group:portfolio-test" }
  }

  mock_resource "aws_acm_certificate" {
    defaults = {
      arn = "arn:aws:acm:us-west-2:111122223333:certificate/00000000-0000-0000-0000-000000000000"
      domain_validation_options = [
        {
          domain_name           = "dev.craigdevjohnson.com"
          resource_record_name  = "_dev.craigdevjohnson.com"
          resource_record_type  = "CNAME"
          resource_record_value = "_dev.acm-validations.aws"
        },
      ]
    }
  }

  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = { arn = "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test" }
  }

  mock_resource "aws_dynamodb_table" {
    defaults = { arn = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test" }
  }

  mock_resource "aws_sqs_queue" {
    defaults = { arn = "arn:aws:sqs:us-west-2:111122223333:portfolio-lambda-dev-soccer-history-failures" }
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

  mock_resource "aws_apigatewayv2_domain_name" {
    defaults = {
      domain_name_configuration = {
        target_domain_name = "example.execute-api.us-west-2.amazonaws.com"
      }
    }
  }

  mock_resource "aws_lambda_function" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev/invocations"
      version    = "1"
    }
  }

  mock_resource "aws_lambda_alias" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev:live"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev:live/invocations"
    }
  }
}

variables {
  aws_account_id = "111122223333"
}

run "development_environment_contract" {
  command = plan

  assert {
    condition = (
      var.environment == "dev" &&
      var.name_prefix == "portfolio-lambda-dev" &&
      var.aws_region == "us-west-2" &&
      var.ecr_repository_url == "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases" &&
      var.image_digest == "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      var.lambda_memory_mb == 512 &&
      var.lambda_timeout_seconds == 29 &&
      var.reserved_concurrency == -1 &&
      var.log_retention_days == 14 &&
      !var.enable_pitr &&
      !var.enable_deletion_protection &&
      length(var.alarm_action_arns) == 0 &&
      toset(var.domain_names) == toset(["dev.craigdevjohnson.com"]) &&
      var.request_custom_domain &&
      var.activate_custom_domain
    )
    error_message = "development must use the reviewed isolated environment values"
  }

  assert {
    condition = (
      output.environment == "dev" &&
      output.image_uri == "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      output.lambda_function_name == "portfolio-lambda-dev" &&
      output.lambda_function_arn == "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev" &&
      output.lambda_published_version == "1" &&
      output.lambda_alias_name == "live" &&
      output.lambda_alias_arn == "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev:live" &&
      output.api_id == "test-api" &&
      output.api_default_url == "https://test.execute-api.us-west-2.amazonaws.com"
    )
    error_message = "development string outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.lambda_execution_role_name == "portfolio-lambda-dev-execution" &&
      output.lambda_execution_permissions_boundary_arn == "arn:aws:iam::111122223333:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
      output.lambda_runtime_policy_name == "portfolio-lambda-dev-runtime" &&
      output.api_name == "portfolio-lambda-dev-http" &&
      output.alarm_names == tolist([
        "portfolio-lambda-dev-api-5xx",
        "portfolio-lambda-dev-api-latency",
        "portfolio-lambda-dev-lambda-duration",
        "portfolio-lambda-dev-lambda-errors",
        "portfolio-lambda-dev-lambda-throttles",
      ])
    )
    error_message = "development deployment names and execution boundary must remain deterministic"
  }

  assert {
    condition = (
      output.lambda_log_group_name == "/aws/lambda/portfolio-lambda-dev" &&
      output.api_access_log_group_name == "/aws/apigateway/portfolio-lambda-dev/access" &&
      output.google_connection_table_name == "portfolio-lambda-dev-google-connections" &&
      output.google_connection_table_arn == "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test" &&
      output.soccer_session_table_name == "portfolio-lambda-dev-soccer-sessions" &&
      output.soccer_session_table_arn == "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test"
    )
    error_message = "development storage and log outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.soccer_history_table_name == null &&
      output.soccer_history_table_arn == null &&
      output.soccer_history_worker_function_name == null &&
      output.soccer_history_schedule_name == null
    )
    error_message = "development must not plan the durable Soccer history table, worker or schedule before its activation review"
  }

  # Gates 3 and 4 of the LPS history readiness packet accepted these limits on
  # September 30, 2026, and Craig decided the same day that development neither
  # collects nor runs the daily schedule, so every switch is off.
  assert {
    condition = (
      !var.enable_soccer_history &&
      !var.activate_soccer_history_collection &&
      !var.activate_soccer_history_schedule &&
      var.soccer_history_schedule_expression == null &&
      var.soccer_history_limits == {
        max_enrolled_teams      = 40
        reserved_player_slots   = 30
        max_requests_per_run    = 120
        max_retries_per_team    = 1
        min_request_interval_ms = 1000
        worker_timeout_seconds  = 300
      }
    )
    error_message = "development must carry the accepted history limits with collection and the schedule off"
  }

  assert {
    condition = output.ssm_parameter_paths == tomap({
      CLIENT_ID_KEY     = "/portfolio/lambda/dev/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY = "/portfolio/lambda/dev/CLIENT_SECRET_KEY"
      LPS_SESSION_KEY   = "/portfolio/lambda/dev/LPS_SESSION_KEY"
      SITE_SESSION_KEY  = "/portfolio/lambda/dev/SITE_SESSION_KEY"
    })
    error_message = "development must expose only its four SSM paths, including SITE_SESSION_KEY now that its reviewed site identity is set"
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
      length(output.acm_validation_records) == 1 &&
      output.acm_validation_records[0].domain_name == "dev.craigdevjohnson.com" &&
      output.acm_validation_records[0].resource_record_name == "_dev.craigdevjohnson.com" &&
      output.acm_validation_records[0].resource_record_type == "CNAME" &&
      output.acm_validation_records[0].resource_record_value == "_dev.acm-validations.aws" &&
      output.api_gateway_domain_targets == tomap({
        "dev.craigdevjohnson.com" = "example.execute-api.us-west-2.amazonaws.com"
      }) &&
      output.oauth_redirect_uris == tolist(["https://dev.craigdevjohnson.com/soccer"])
    )
    error_message = "development custom-domain outputs must expose DNS validation and the active Regional API target"
  }
}

run "management_runtime_contract" {
  command = plan
  variables {
    management = {
      aws_region = "us-west-2"
    }
  }
  assert {
    condition = (
      output.ssm_parameter_paths == tomap({
        CLIENT_ID_KEY     = "/portfolio/lambda/dev/CLIENT_ID_KEY"
        CLIENT_SECRET_KEY = "/portfolio/lambda/dev/CLIENT_SECRET_KEY"
        LPS_SESSION_KEY   = "/portfolio/lambda/dev/LPS_SESSION_KEY"
        SITE_SESSION_KEY  = "/portfolio/lambda/dev/SITE_SESSION_KEY"
      }) &&
      output.lambda_execution_role_name == "portfolio-lambda-dev-execution"
    )
    error_message = "dev root must forward the identity-free management switch into its existing runtime without the retired MGMT_SESSION_KEY parameter"
  }
}

run "reject_management_in_another_region" {
  command = plan
  variables {
    management = {
      aws_region = "us-east-1"
    }
  }
  expect_failures = [var.management]
}

run "site_runtime_contract" {
  command = plan

  variables {
    site = {
      cognito_domain       = "https://portfolio-lambda-dev-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_DevSite"
      cognito_client_id    = "devsiteclient"
      redirect_uri         = "https://dev.craigdevjohnson.com/auth/callback"
      logout_uri           = "https://dev.craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer", "management"] }
      allow_local_callback = false
    }
  }

  assert {
    condition     = output.ssm_parameter_paths.SITE_SESSION_KEY == "/portfolio/lambda/dev/SITE_SESSION_KEY"
    error_message = "development must forward its site identity with a development-only session parameter path"
  }
}

# Proves the root hands every history input from dev.auto.tfvars to the
# service module: switching them on here plans each stage. This is not a
# reviewed development configuration.
run "history_inputs_reach_the_service" {
  command = plan

  variables {
    alarm_action_arns                  = ["arn:aws:sns:us-west-2:111122223333:alerts"]
    enable_soccer_history              = true
    activate_soccer_history_collection = true
    activate_soccer_history_schedule   = true
    soccer_history_schedule_expression = "cron(30 10 * * ? *)"
  }

  assert {
    condition = (
      output.soccer_history_table_name == "portfolio-lambda-dev-soccer-history" &&
      output.soccer_history_worker_function_name == "portfolio-lambda-dev-soccer-history" &&
      output.soccer_history_schedule_name == "portfolio-lambda-dev-soccer-history-daily" &&
      output.alarm_names == tolist([
        "portfolio-lambda-dev-api-5xx",
        "portfolio-lambda-dev-api-latency",
        "portfolio-lambda-dev-lambda-duration",
        "portfolio-lambda-dev-lambda-errors",
        "portfolio-lambda-dev-lambda-throttles",
        "portfolio-lambda-dev-soccer-history-admission-rejected",
        "portfolio-lambda-dev-soccer-history-dead-letter",
        "portfolio-lambda-dev-soccer-history-errors",
        "portfolio-lambda-dev-soccer-history-incomplete",
      ])
    )
    error_message = "development must pass the history switches, limits and schedule to the service module"
  }
}
