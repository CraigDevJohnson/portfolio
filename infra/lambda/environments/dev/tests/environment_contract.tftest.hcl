mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = { account_id = "180294223248" }
  }

  mock_data "aws_partition" {
    defaults = { partition = "aws" }
  }

  mock_data "aws_kms_alias" {
    defaults = { target_key_arn = "arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000" }
  }

  mock_data "aws_iam_policy_document" {
    defaults = { json = "{}" }
  }

  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-west-2:180294223248:log-group:portfolio-test" }
  }

  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = { arn = "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test" }
  }

  mock_resource "aws_dynamodb_table" {
    defaults = { arn = "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test" }
  }

  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::180294223248:role/portfolio-lambda-test" }
  }

  mock_resource "aws_apigatewayv2_api" {
    defaults = {
      api_endpoint  = "https://test.execute-api.us-west-2.amazonaws.com"
      execution_arn = "arn:aws:execute-api:us-west-2:180294223248:test-api"
      id            = "test-api"
    }
  }

  mock_resource "aws_lambda_function" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev/invocations"
      version    = "1"
    }
  }

  mock_resource "aws_lambda_alias" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev:live"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev:live/invocations"
    }
  }
}

run "development_environment_contract" {
  command = plan

  assert {
    condition = (
      var.environment == "dev" &&
      var.name_prefix == "portfolio-lambda-dev" &&
      var.aws_region == "us-west-2" &&
      var.ecr_repository_url == "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases" &&
      var.image_digest == "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      var.lambda_memory_mb == 512 &&
      var.lambda_timeout_seconds == 29 &&
      var.reserved_concurrency == 5 &&
      var.log_retention_days == 14 &&
      !var.enable_pitr &&
      !var.enable_deletion_protection &&
      length(var.alarm_action_arns) == 0 &&
      toset(var.domain_names) == toset(["dev.craigdevjohnson.com"]) &&
      !var.request_custom_domain &&
      !var.activate_custom_domain
    )
    error_message = "development must use the reviewed isolated environment values"
  }

  assert {
    condition = (
      output.environment == "dev" &&
      output.image_uri == "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      output.lambda_function_name == "portfolio-lambda-dev" &&
      output.lambda_function_arn == "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev" &&
      output.lambda_published_version == "1" &&
      output.lambda_alias_name == "live" &&
      output.lambda_alias_arn == "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev:live" &&
      output.api_id == "test-api" &&
      output.api_default_url == "https://test.execute-api.us-west-2.amazonaws.com"
    )
    error_message = "development string outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.lambda_execution_role_name == "portfolio-lambda-dev-execution" &&
      output.lambda_execution_permissions_boundary_arn == "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
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
      output.google_connection_table_arn == "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test" &&
      output.soccer_session_table_name == "portfolio-lambda-dev-soccer-sessions" &&
      output.soccer_session_table_arn == "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test"
    )
    error_message = "development storage and log outputs must forward the evaluated service values"
  }

  assert {
    condition = output.ssm_parameter_paths == tomap({
      CLIENT_ID_KEY     = "/portfolio/lambda/dev/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY = "/portfolio/lambda/dev/CLIENT_SECRET_KEY"
      LPS_SESSION_KEY   = "/portfolio/lambda/dev/LPS_SESSION_KEY"
    })
    error_message = "development must expose only the three non-secret SSM paths"
  }

  assert {
    condition = (
      output.alarm_arns == tolist([
        "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test",
        "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test",
      ]) &&
      output.certificate_arn == tostring(null) &&
      length(output.acm_validation_records) == 0 &&
      length(output.api_gateway_domain_targets) == 0 &&
      output.oauth_redirect_uris == tolist(["https://dev.craigdevjohnson.com/soccer"])
    )
    error_message = "development collection and nullable outputs must keep their reviewed values and shapes"
  }
}
