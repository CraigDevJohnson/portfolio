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

  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = {
      arn = "arn:aws:cloudwatch:us-west-2:180294223248:alarm:portfolio-test"
    }
  }

  mock_resource "aws_dynamodb_table" {
    defaults = {
      arn = "arn:aws:dynamodb:us-west-2:180294223248:table/portfolio-test"
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
  alarm_action_arns          = ["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-alerts"]
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
    condition = (
      aws_lambda_function.app.architectures == tolist(["x86_64"]) &&
      aws_lambda_function.app.package_type == "Image" &&
      aws_lambda_function.app.memory_size == 512 &&
      aws_lambda_function.app.timeout == 29 &&
      aws_lambda_function.app.reserved_concurrent_executions == 5 &&
      aws_lambda_function.app.publish
    )
    error_message = "the Lambda runtime must use the reviewed image, architecture, sizing, and published-version settings"
  }

  assert {
    condition = (
      aws_lambda_alias.live.name == "live" &&
      aws_lambda_alias.live.function_name == aws_lambda_function.app.function_name &&
      aws_lambda_alias.live.function_version == aws_lambda_function.app.version
    )
    error_message = "the live alias must select the newly published function version by default"
  }

  assert {
    condition = (
      aws_apigatewayv2_api.app.protocol_type == "HTTP" &&
      aws_apigatewayv2_api.app.name == "portfolio-lambda-dev-http" &&
      aws_apigatewayv2_integration.lambda.integration_type == "AWS_PROXY" &&
      aws_apigatewayv2_integration.lambda.integration_uri == aws_lambda_alias.live.invoke_arn &&
      aws_apigatewayv2_integration.lambda.payload_format_version == "2.0" &&
      aws_apigatewayv2_route.default.route_key == "$default" &&
      aws_apigatewayv2_route.default.target == "integrations/${aws_apigatewayv2_integration.lambda.id}" &&
      aws_apigatewayv2_stage.default.name == "$default" &&
      aws_apigatewayv2_stage.default.auto_deploy
    )
    error_message = "the HTTP API must use an auto-deployed default route with a payload-v2 alias integration"
  }

  assert {
    condition = (
      aws_iam_role.lambda.name == "portfolio-lambda-dev-execution" &&
      aws_iam_role.lambda.permissions_boundary == "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
      aws_iam_role_policy.lambda.name == "portfolio-lambda-dev-runtime"
    )
    error_message = "runtime IAM must use the deterministic names and root-owned execution boundary"
  }

  assert {
    condition = (
      aws_lambda_permission.api.action == "lambda:InvokeFunction" &&
      aws_lambda_permission.api.function_name == aws_lambda_function.app.function_name &&
      aws_lambda_permission.api.qualifier == aws_lambda_alias.live.name &&
      aws_lambda_permission.api.principal == "apigateway.amazonaws.com" &&
      aws_lambda_permission.api.source_arn == "${aws_apigatewayv2_api.app.execution_arn}/*/*"
    )
    error_message = "API Gateway permission must invoke only the live alias from the reviewed API execution ARN"
  }

  assert {
    condition = (
      aws_apigatewayv2_stage.default.access_log_settings[0].destination_arn == aws_cloudwatch_log_group.api_access.arn &&
      jsondecode(aws_apigatewayv2_stage.default.access_log_settings[0].format) == {
        integration_error_message = "$context.integrationErrorMessage"
        integration_latency       = "$context.integrationLatency"
        integration_status        = "$context.integrationStatus"
        method                    = "$context.httpMethod"
        path                      = "$context.path"
        request_id                = "$context.requestId"
        response_latency          = "$context.responseLatency"
        response_length           = "$context.responseLength"
        route_key                 = "$context.routeKey"
        source_ip                 = "$context.identity.sourceIp"
        status                    = "$context.status"
      }
    )
    error_message = "API access logs must contain exactly the reviewed metadata fields and no request content"
  }

  assert {
    condition     = aws_cloudwatch_log_group.lambda.retention_in_days == 14 && aws_cloudwatch_log_group.api_access.retention_in_days == 14
    error_message = "both service log groups must use finite configured retention"
  }

  assert {
    condition = (
      length(data.aws_iam_policy_document.lambda.statement) == 5 &&
      alltrue([
        for statement in data.aws_iam_policy_document.lambda.statement :
        (statement.effect == null || statement.effect == "Allow") &&
        statement.not_actions == null &&
        statement.not_resources == null &&
        length(statement.condition) == 0 &&
        length(statement.principals) == 0 &&
        length(statement.not_principals) == 0
      ]) &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 3 &&
        toset(statement.actions) == toset(["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset([aws_dynamodb_table.google_connections.arn])
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 1 &&
        toset(statement.actions) == toset(["dynamodb:PutItem"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset([aws_dynamodb_table.soccer_sessions.arn])
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 1 &&
        toset(statement.actions) == toset(["ssm:GetParameters"]) &&
        length(statement.resources) == 3 &&
        toset(statement.resources) == toset([
          "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/CLIENT_ID_KEY",
          "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/CLIENT_SECRET_KEY",
          "arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/LPS_SESSION_KEY",
        ])
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 1 &&
        toset(statement.actions) == toset(["kms:Decrypt"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset(["arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000"])
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 2 &&
        toset(statement.actions) == toset(["logs:CreateLogStream", "logs:PutLogEvents"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset(["${aws_cloudwatch_log_group.lambda.arn}:*"])
      ]) == 1
    )
    error_message = "runtime IAM must contain exactly the reviewed Google, Soccer, SSM, KMS, and Lambda log statements"
  }

  assert {
    condition = (
      aws_cloudwatch_metric_alarm.lambda_errors.metric_name == "Errors" &&
      aws_cloudwatch_metric_alarm.lambda_errors.alarm_name == "portfolio-lambda-dev-lambda-errors" &&
      aws_cloudwatch_metric_alarm.lambda_errors.namespace == "AWS/Lambda" &&
      aws_cloudwatch_metric_alarm.lambda_errors.period == 300 &&
      aws_cloudwatch_metric_alarm.lambda_errors.evaluation_periods == 1 &&
      aws_cloudwatch_metric_alarm.lambda_errors.comparison_operator == "GreaterThanOrEqualToThreshold" &&
      aws_cloudwatch_metric_alarm.lambda_errors.statistic == "Sum" &&
      aws_cloudwatch_metric_alarm.lambda_errors.threshold == 1 &&
      aws_cloudwatch_metric_alarm.lambda_errors.dimensions == tomap({ FunctionName = "portfolio-lambda-dev" }) &&
      aws_cloudwatch_metric_alarm.lambda_throttles.metric_name == "Throttles" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.alarm_name == "portfolio-lambda-dev-lambda-throttles" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.namespace == "AWS/Lambda" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.period == 300 &&
      aws_cloudwatch_metric_alarm.lambda_throttles.evaluation_periods == 1 &&
      aws_cloudwatch_metric_alarm.lambda_throttles.comparison_operator == "GreaterThanOrEqualToThreshold" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.statistic == "Sum" &&
      aws_cloudwatch_metric_alarm.lambda_throttles.threshold == 1 &&
      aws_cloudwatch_metric_alarm.lambda_throttles.dimensions == tomap({ FunctionName = "portfolio-lambda-dev" }) &&
      aws_cloudwatch_metric_alarm.lambda_duration.metric_name == "Duration" &&
      aws_cloudwatch_metric_alarm.lambda_duration.alarm_name == "portfolio-lambda-dev-lambda-duration" &&
      aws_cloudwatch_metric_alarm.lambda_duration.namespace == "AWS/Lambda" &&
      aws_cloudwatch_metric_alarm.lambda_duration.period == 300 &&
      aws_cloudwatch_metric_alarm.lambda_duration.evaluation_periods == 1 &&
      aws_cloudwatch_metric_alarm.lambda_duration.comparison_operator == "GreaterThanOrEqualToThreshold" &&
      aws_cloudwatch_metric_alarm.lambda_duration.extended_statistic == "p95" &&
      aws_cloudwatch_metric_alarm.lambda_duration.threshold == 24000 &&
      aws_cloudwatch_metric_alarm.lambda_duration.dimensions == tomap({ FunctionName = "portfolio-lambda-dev" }) &&
      aws_cloudwatch_metric_alarm.api_5xx.metric_name == "5xx" &&
      aws_cloudwatch_metric_alarm.api_5xx.alarm_name == "portfolio-lambda-dev-api-5xx" &&
      aws_cloudwatch_metric_alarm.api_5xx.namespace == "AWS/ApiGateway" &&
      aws_cloudwatch_metric_alarm.api_5xx.period == 300 &&
      aws_cloudwatch_metric_alarm.api_5xx.evaluation_periods == 1 &&
      aws_cloudwatch_metric_alarm.api_5xx.comparison_operator == "GreaterThanOrEqualToThreshold" &&
      aws_cloudwatch_metric_alarm.api_5xx.statistic == "Sum" &&
      aws_cloudwatch_metric_alarm.api_5xx.threshold == 1 &&
      aws_cloudwatch_metric_alarm.api_5xx.dimensions == tomap({ ApiId = "test-api" }) &&
      aws_cloudwatch_metric_alarm.api_latency.metric_name == "Latency" &&
      aws_cloudwatch_metric_alarm.api_latency.alarm_name == "portfolio-lambda-dev-api-latency" &&
      aws_cloudwatch_metric_alarm.api_latency.namespace == "AWS/ApiGateway" &&
      aws_cloudwatch_metric_alarm.api_latency.period == 300 &&
      aws_cloudwatch_metric_alarm.api_latency.evaluation_periods == 1 &&
      aws_cloudwatch_metric_alarm.api_latency.comparison_operator == "GreaterThanOrEqualToThreshold" &&
      aws_cloudwatch_metric_alarm.api_latency.extended_statistic == "p95" &&
      aws_cloudwatch_metric_alarm.api_latency.threshold == 25000 &&
      aws_cloudwatch_metric_alarm.api_latency.dimensions == tomap({ ApiId = "test-api" }) &&
      alltrue([
        for alarm in [
          aws_cloudwatch_metric_alarm.lambda_errors,
          aws_cloudwatch_metric_alarm.lambda_throttles,
          aws_cloudwatch_metric_alarm.lambda_duration,
          aws_cloudwatch_metric_alarm.api_5xx,
          aws_cloudwatch_metric_alarm.api_latency,
        ] : alarm.treat_missing_data == "notBreaching" && toset(alarm.alarm_actions) == toset(["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-alerts"])
      ])
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

run "live_version_override_contract" {
  command = plan

  variables {
    live_version_override = 7
  }

  assert {
    condition     = aws_lambda_alias.live.function_version == "7"
    error_message = "live_version_override must select the requested published version"
  }
}

run "certificate_request_only_contract" {
  command = plan

  variables {
    request_custom_domain = true
  }

  assert {
    condition = (
      length(aws_acm_certificate.custom) == 1 &&
      length(aws_acm_certificate_validation.custom) == 0 &&
      length(aws_apigatewayv2_domain_name.custom) == 0 &&
      length(aws_apigatewayv2_api_mapping.custom) == 0
    )
    error_message = "requesting a certificate must not activate validation or API custom-domain resources"
  }

  assert {
    condition = (
      output.certificate_arn == "arn:aws:acm:us-west-2:180294223248:certificate/00000000-0000-0000-0000-000000000000" &&
      [for record in output.acm_validation_records : record.domain_name] == ["api.example.com", "www.example.com"] &&
      length(output.api_gateway_domain_targets) == 0
    )
    error_message = "certificate request outputs must expose sorted DNS records without active API targets"
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
