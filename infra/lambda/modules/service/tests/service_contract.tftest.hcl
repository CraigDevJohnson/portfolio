mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "180294223248"
    }
  }

  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }

  mock_data "aws_kms_alias" {
    defaults = {
      target_key_arn = "arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000"
    }
  }

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{}"
    }
  }

  mock_resource "aws_acm_certificate" {
    defaults = {
      arn = "arn:aws:acm:us-west-2:180294223248:certificate/00000000-0000-0000-0000-000000000000"
      domain_validation_options = [
        {
          domain_name           = "api.example.com"
          resource_record_name  = "_api.example.com"
          resource_record_type  = "CNAME"
          resource_record_value = "_api.acm-validations.aws"
        },
        {
          domain_name           = "www.example.com"
          resource_record_name  = "_www.example.com"
          resource_record_type  = "CNAME"
          resource_record_value = "_www.acm-validations.aws"
        },
      ]
    }
  }

  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-west-2:180294223248:log-group:portfolio-test"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::180294223248:role/portfolio-lambda-test"
    }
  }

  mock_resource "aws_apigatewayv2_domain_name" {
    defaults = {
      domain_name_configuration = {
        target_domain_name = "example.execute-api.us-west-2.amazonaws.com"
      }
    }
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

variables {
  environment                = "dev"
  name_prefix                = "portfolio-lambda-dev"
  aws_region                 = "us-west-2"
  ecr_repository_url         = "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases"
  image_digest               = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  lambda_memory_mb           = 512
  lambda_timeout_seconds     = 29
  reserved_concurrency       = 5
  log_retention_days         = 14
  enable_pitr                = false
  enable_deletion_protection = false
  alarm_action_arns          = []
  domain_names               = ["www.example.com", "api.example.com"]
  request_custom_domain      = false
  activate_custom_domain     = false
}

run "published_service_contract" {
  command = plan

  assert {
    condition     = aws_lambda_function.app.image_uri == "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    error_message = "the Lambda image must use the supplied immutable digest"
  }

  assert {
    condition     = aws_lambda_function.app.publish && aws_lambda_alias.live.name == "live"
    error_message = "the function must publish a version behind the live alias"
  }

  assert {
    condition     = aws_apigatewayv2_integration.lambda.integration_uri == aws_lambda_alias.live.invoke_arn && aws_lambda_permission.api.qualifier == aws_lambda_alias.live.name
    error_message = "API Gateway and its permission must target only the live alias"
  }

  assert {
    condition     = aws_cloudwatch_log_group.lambda.retention_in_days == 14 && aws_cloudwatch_log_group.api_access.retention_in_days == 14
    error_message = "both service log groups must use finite configured retention"
  }

  assert {
    condition = (
      !contains(flatten([for statement in data.aws_iam_policy_document.lambda.statement : statement.actions]), "logs:CreateLogGroup") &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if toset(statement.actions) == toset(["logs:CreateLogStream", "logs:PutLogEvents"]) &&
        toset(statement.resources) == toset(["${aws_cloudwatch_log_group.lambda.arn}:*"])
      ]) == 1
    )
    error_message = "runtime IAM must write only to streams in the precreated Lambda log group"
  }

  assert {
    condition = (
      aws_cloudwatch_metric_alarm.lambda_errors.metric_name == "Errors" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.metric_name == "Throttles" &&
      aws_cloudwatch_metric_alarm.lambda_duration.metric_name == "Duration" &&
      aws_cloudwatch_metric_alarm.lambda_duration.extended_statistic == "p95" &&
      aws_cloudwatch_metric_alarm.lambda_duration.threshold == 24000 &&
      aws_cloudwatch_metric_alarm.api_5xx.metric_name == "5xx" &&
      aws_cloudwatch_metric_alarm.api_latency.metric_name == "Latency" &&
      aws_cloudwatch_metric_alarm.api_latency.extended_statistic == "p95" &&
      aws_cloudwatch_metric_alarm.api_latency.threshold == 25000
    )
    error_message = "the service must define the five required Lambda and API alarms"
  }

  assert {
    condition     = length(output.alarm_arns) == 5 && output.alarm_arns == sort(output.alarm_arns)
    error_message = "alarm_arns must be a sorted list containing all five alarms"
  }

  assert {
    condition     = output.environment == "dev" && output.image_uri == aws_lambda_function.app.image_uri && output.lambda_alias_name == "live"
    error_message = "string outputs must expose the configured service and published alias"
  }

  assert {
    condition     = toset(keys(output.ssm_parameter_paths)) == toset(["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"])
    error_message = "ssm_parameter_paths must be keyed by the three application variable names"
  }

  assert {
    condition     = output.certificate_arn == null && length(output.acm_validation_records) == 0 && length(output.api_gateway_domain_targets) == 0
    error_message = "custom-domain resources and outputs must remain inactive by default"
  }

  assert {
    condition     = output.oauth_redirect_uris == tolist(["https://api.example.com/soccer", "https://www.example.com/soccer"])
    error_message = "OAuth redirect URIs must be a sorted list derived from the requested domains"
  }
}

run "staged_custom_domain_contract" {
  command = plan

  variables {
    request_custom_domain  = true
    activate_custom_domain = true
  }

  assert {
    condition     = aws_acm_certificate.custom[0].domain_name == "api.example.com" && aws_acm_certificate.custom[0].subject_alternative_names == toset(["www.example.com"])
    error_message = "the certificate request must use the sorted first domain and remaining SANs"
  }

  assert {
    condition     = [for record in output.acm_validation_records : record.domain_name] == ["api.example.com", "www.example.com"]
    error_message = "ACM validation records must be stable and sorted by domain"
  }

  assert {
    condition     = toset(keys(output.api_gateway_domain_targets)) == toset(["api.example.com", "www.example.com"])
    error_message = "activated custom domains must expose one Regional API target per hostname"
  }
}
