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
