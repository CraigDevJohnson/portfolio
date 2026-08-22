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
      arn        = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod/invocations"
      version    = "1"
    }
  }

  mock_resource "aws_lambda_alias" {
    defaults = {
      arn        = "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod:live"
      invoke_arn = "arn:aws:apigateway:us-west-2:lambda:path/2015-03-31/functions/arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod:live/invocations"
    }
  }
}

run "production_environment_contract" {
  command = plan

  assert {
    condition = (
      var.environment == "prod" &&
      var.name_prefix == "portfolio-lambda-prod" &&
      var.aws_region == "us-west-2" &&
      var.ecr_repository_url == "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases" &&
      var.image_digest == "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      var.lambda_memory_mb == 512 &&
      var.lambda_timeout_seconds == 29 &&
      var.reserved_concurrency == 10 &&
      var.log_retention_days == 90 &&
      var.enable_pitr &&
      var.enable_deletion_protection &&
      toset(jsondecode(var.alarm_action_arns)) == toset(["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]) &&
      toset(var.domain_names) == toset(["craigdevjohnson.com", "www.craigdevjohnson.com"]) &&
      !var.request_custom_domain &&
      !var.activate_custom_domain
    )
    error_message = "production must use the reviewed isolated environment values"
  }

  assert {
    condition = (
      output.environment == "prod" &&
      output.image_uri == "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" &&
      output.lambda_function_name == "portfolio-lambda-prod" &&
      output.lambda_function_arn == "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod" &&
      output.lambda_published_version == "1" &&
      output.lambda_alias_name == "live" &&
      output.lambda_alias_arn == "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-prod:live" &&
      output.api_id == "test-api" &&
      output.api_default_url == "https://test.execute-api.us-west-2.amazonaws.com"
    )
    error_message = "production string outputs must forward the evaluated service values"
  }

  assert {
    condition = (
      output.lambda_log_group_name == "/aws/lambda/portfolio-lambda-prod" &&
      output.api_access_log_group_name == "/aws/apigateway/portfolio-lambda-prod/access" &&
      output.google_connection_table_name == "portfolio-lambda-prod-google-connections" &&
      output.google_connection_table_arn == "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test" &&
      output.soccer_session_table_name == "portfolio-lambda-prod-soccer-sessions" &&
      output.soccer_session_table_arn == "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test"
    )
    error_message = "production storage and log outputs must forward the evaluated service values"
  }

  assert {
    condition = output.ssm_parameter_paths == tomap({
      CLIENT_ID_KEY     = "/portfolio/lambda/prod/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY = "/portfolio/lambda/prod/CLIENT_SECRET_KEY"
      LPS_SESSION_KEY   = "/portfolio/lambda/prod/LPS_SESSION_KEY"
    })
    error_message = "production must expose only the three non-secret SSM paths"
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
      output.oauth_redirect_uris == tolist([
        "https://craigdevjohnson.com/soccer",
        "https://www.craigdevjohnson.com/soccer",
      ])
    )
    error_message = "production collection and nullable outputs must keep their reviewed values and shapes"
  }
}
