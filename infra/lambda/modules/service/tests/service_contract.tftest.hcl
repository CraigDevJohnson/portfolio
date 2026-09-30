mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "111122223333"
    }
  }

  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }

  mock_data "aws_kms_alias" {
    defaults = {
      target_key_arn = "arn:aws:kms:us-west-2:111122223333:key/00000000-0000-0000-0000-000000000000"
    }
  }

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{}"
    }
  }

  mock_resource "aws_acm_certificate" {
    defaults = {
      arn = "arn:aws:acm:us-west-2:111122223333:certificate/00000000-0000-0000-0000-000000000000"
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
      arn = "arn:aws:logs:us-west-2:111122223333:log-group:portfolio-test"
    }
  }

  mock_resource "aws_cloudwatch_metric_alarm" {
    defaults = {
      arn = "arn:aws:cloudwatch:us-west-2:111122223333:alarm:portfolio-test"
    }
  }

  mock_resource "aws_dynamodb_table" {
    defaults = {
      arn = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-test"
    }
  }

  mock_resource "aws_sqs_queue" {
    defaults = {
      arn = "arn:aws:sqs:us-west-2:111122223333:portfolio-test-history-failures"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::111122223333:role/portfolio-lambda-test"
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
      execution_arn = "arn:aws:execute-api:us-west-2:111122223333:test-api"
      id            = "test-api"
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
  environment                = "dev"
  name_prefix                = "portfolio-lambda-dev"
  aws_region                 = "us-west-2"
  ecr_repository_url         = "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases"
  image_digest               = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  lambda_memory_mb           = 512
  lambda_timeout_seconds     = 29
  reserved_concurrency       = -1
  log_retention_days         = 14
  enable_pitr                = false
  enable_deletion_protection = false
  alarm_action_arns          = ["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"]
  domain_names               = ["www.example.com", "api.example.com"]
  request_custom_domain      = false
  activate_custom_domain     = false
}

run "published_service_contract" {
  command = plan

  assert {
    condition     = aws_lambda_function.app.image_uri == "111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    error_message = "the Lambda image must use the supplied immutable digest"
  }

  assert {
    condition = (
      aws_lambda_function.app.architectures == tolist(["x86_64"]) &&
      aws_lambda_function.app.package_type == "Image" &&
      aws_lambda_function.app.memory_size == 512 &&
      aws_lambda_function.app.timeout == 29 &&
      aws_lambda_function.app.reserved_concurrent_executions == -1 &&
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
      aws_iam_role.lambda.permissions_boundary == "arn:aws:iam::111122223333:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
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
      length(aws_dynamodb_table.soccer_history) == 0 &&
      output.soccer_history_table_name == null &&
      output.soccer_history_table_arn == null
    )
    error_message = "durable Soccer history must not be planned until its activation review enables it"
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
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_ID_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_SECRET_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/LPS_SESSION_KEY",
        ])
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 1 &&
        toset(statement.actions) == toset(["kms:Decrypt"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset(["arn:aws:kms:us-west-2:111122223333:key/00000000-0000-0000-0000-000000000000"])
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
      data.aws_iam_policy_document.lambda.source_policy_documents == null &&
      data.aws_iam_policy_document.lambda.override_policy_documents == null
    )
    error_message = "the reviewed runtime document must not merge source or override policies"
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
        ] : alarm.treat_missing_data == "notBreaching" && toset(alarm.alarm_actions) == toset(["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"])
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
      output.certificate_arn == "arn:aws:acm:us-west-2:111122223333:certificate/00000000-0000-0000-0000-000000000000" &&
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

  assert {
    condition     = alltrue([for domain in aws_apigatewayv2_domain_name.custom : domain.routing_mode == "API_MAPPING_ONLY"])
    error_message = "custom domains must route through the declared API mappings"
  }
}

run "soccer_history_enabled_contract" {
  command = plan

  variables {
    enable_soccer_history      = true
    enable_pitr                = true
    enable_deletion_protection = true
  }

  # Every mocked table shares one ARN; give this table its own so the IAM
  # checks below can tell its grant apart from the other tables'.
  override_resource {
    target = aws_dynamodb_table.soccer_history
    values = {
      arn = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-lambda-dev-soccer-history"
    }
  }

  assert {
    condition = (
      length(aws_dynamodb_table.soccer_history) == 1 &&
      aws_dynamodb_table.soccer_history[0].name == "portfolio-lambda-dev-soccer-history" &&
      aws_dynamodb_table.soccer_history[0].billing_mode == "PAY_PER_REQUEST" &&
      aws_dynamodb_table.soccer_history[0].hash_key == "pk" &&
      aws_dynamodb_table.soccer_history[0].range_key == "sk" &&
      length(aws_dynamodb_table.soccer_history[0].ttl) == 0 &&
      aws_dynamodb_table.soccer_history[0].server_side_encryption[0].enabled &&
      aws_dynamodb_table.soccer_history[0].point_in_time_recovery[0].enabled &&
      aws_dynamodb_table.soccer_history[0].deletion_protection_enabled &&
      length(aws_dynamodb_table.soccer_history[0].global_secondary_index) == 1 &&
      length([
        for index in aws_dynamodb_table.soccer_history[0].global_secondary_index : index
        if index.name == "due-teams" && index.projection_type == "ALL" &&
        join(",", [for key in index.key_schema : "${key.attribute_name}:${key.key_type}"]) == "due_pk:HASH,due_sk:RANGE"
      ]) == 1 &&
      output.soccer_history_table_name == "portfolio-lambda-dev-soccer-history" &&
      output.soccer_history_table_arn == "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-lambda-dev-soccer-history"
    )
    error_message = "enabled Soccer history must be a protected, encrypted source-fact table with a due-team index and no session TTL"
  }

  assert {
    condition = (
      length(data.aws_iam_policy_document.lambda.statement) == 6 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if length(statement.actions) == 4 &&
        toset(statement.actions) == toset(["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query", "dynamodb:DeleteItem"]) &&
        length(statement.resources) == 1 &&
        toset(statement.resources) == toset([aws_dynamodb_table.soccer_history[0].arn]) &&
        length(statement.condition) == 0
      ]) == 1 &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if contains(statement.resources, aws_dynamodb_table.soccer_history[0].arn)
      ]) == 1
    )
    error_message = "enabled Soccer history must add exactly one read, write, base-table query, and player-removal delete statement for its table"
  }

  assert {
    condition     = !contains(keys(aws_lambda_function.app.environment[0].variables), "SOCCER_ARCHIVE_TABLE_NAME")
    error_message = "enabling the table must not hand the runtime an archive to enroll into before activation"
  }
}

run "runtime_policy_attachment_contract" {
  command = apply

  assert {
    condition = (
      aws_iam_role_policy.lambda.policy == data.aws_iam_policy_document.lambda.json &&
      data.aws_iam_policy_document.lambda.source_policy_documents == null &&
      data.aws_iam_policy_document.lambda.override_policy_documents == null
    )
    error_message = "the attached runtime policy must be only the reviewed document without source or override merges"
  }
}

run "management_enabled_contract" {
  command = plan
  variables {
    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  assert {
    condition = aws_lambda_function.app.environment[0].variables == tomap({
      CLIENT_ID_KEY                = "/portfolio/lambda/dev/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY            = "/portfolio/lambda/dev/CLIENT_SECRET_KEY"
      GOOGLE_CONNECTION_TABLE_NAME = "portfolio-lambda-dev-google-connections"
      LOG_ADD_SOURCE               = "false"
      LOG_FORMAT                   = "json"
      LOG_LEVEL                    = "info"
      LPS_SESSION_KEY              = "/portfolio/lambda/dev/LPS_SESSION_KEY"
      SOCCER_SESSION_TABLE_NAME    = "portfolio-lambda-dev-soccer-sessions"
      MGMT_AWS_REGION              = "us-west-2"
    })
    error_message = "management must pass the portal only its AWS region; the application no longer reads the retired management-only session, Cognito, allowlist or callback settings"
  }
  assert {
    condition = (
      length(data.aws_iam_policy_document.lambda.statement) == 6 &&
      length([for st in data.aws_iam_policy_document.lambda.statement : st if
        toset(st.actions) == toset(["ec2:DescribeInstances", "cloudwatch:GetMetricStatistics"]) &&
        st.resources == toset(["*"]) && length(st.condition) == 1 &&
      alltrue([for c in st.condition : c.test == "StringEquals" && c.variable == "aws:RequestedRegion" && toset(c.values) == toset(["us-west-2"])])]) == 1 &&
      length([for st in data.aws_iam_policy_document.lambda.statement : st if
        length(setintersection(toset(st.actions), toset(["ec2:StartInstances", "ec2:StopInstances", "logs:FilterLogEvents"]))) > 0
      ]) == 0
    )
    error_message = "management must add exactly one read-only, region-scoped statement, with no EC2 start/stop or instance log reads (D22)"
  }
  assert {
    condition = (
      output.ssm_parameter_paths == tomap({
        CLIENT_ID_KEY     = "/portfolio/lambda/dev/CLIENT_ID_KEY"
        CLIENT_SECRET_KEY = "/portfolio/lambda/dev/CLIENT_SECRET_KEY"
        LPS_SESSION_KEY   = "/portfolio/lambda/dev/LPS_SESSION_KEY"
      }) &&
      length([for st in data.aws_iam_policy_document.lambda.statement : st if
        st.actions == toset(["ssm:GetParameters"]) &&
        st.resources == toset([
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_ID_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_SECRET_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/LPS_SESSION_KEY",
      ])]) == 1 &&
      length([for st in data.aws_iam_policy_document.lambda.statement : st if
        st.actions == toset(["kms:Decrypt"]) && length(st.condition) == 1 &&
        alltrue([for c in st.condition : c.test == "StringEquals" && c.variable == "kms:EncryptionContext:PARAMETER_ARN" &&
          toset(c.values) == toset([
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_ID_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/CLIENT_SECRET_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/dev/LPS_SESSION_KEY",
      ])])]) == 1
    )
    error_message = "management adds no SecureString: parameter reads and decryption stay on the three required parameters, without the retired MGMT_SESSION_KEY"
  }
}

run "management_reject_prod" {
  command = plan
  variables {
    environment = "prod"
    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [aws_iam_role.lambda]
}

run "management_reject_region" {
  command = plan
  variables {
    aws_region = "us-east-1"
    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [aws_iam_role.lambda]
}

run "management_reject_email" {
  command = plan
  variables {

    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["other@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [var.management]
}

run "management_reject_empty_email" {
  command = plan
  variables {

    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = []
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [var.management]
}

run "management_reject_callback" {
  command = plan
  variables {

    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "http://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [var.management]
}

run "management_reject_tag" {
  command = plan
  variables {

    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "OtherTag"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [var.management]
}

run "management_reject_issuer" {
  command = plan
  variables {

    management = {
      cognito_domain           = "https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com"
      cognito_issuer           = "https://cognito-idp.us-west-2.amazonaws.com/us-east-1_Test123"
      cognito_client_id        = "testclient123"
      redirect_uri             = "https://dev.craigdevjohnson.com/callback"
      logout_uri               = "https://dev.craigdevjohnson.com/login"
      allowed_emails           = ["craigdevjohnson@gmail.com"]
      allow_local_callback     = false
      ec2_management_tag_key   = "PortfolioManagement"
      ec2_management_tag_value = "dev"
    }
  }
  expect_failures = [var.management]
}

run "development_site_runtime_contract" {
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
    condition = (
      output.ssm_parameter_paths.SITE_SESSION_KEY == "/portfolio/lambda/dev/SITE_SESSION_KEY" &&
      aws_lambda_function.app.environment[0].variables.SITE_SESSION_KEY == "/portfolio/lambda/dev/SITE_SESSION_KEY" &&
      aws_lambda_function.app.environment[0].variables.SITE_COGNITO_ISSUER == "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_DevSite" &&
      aws_lambda_function.app.environment[0].variables.SITE_COGNITO_CLIENT_ID == "devsiteclient" &&
      aws_lambda_function.app.environment[0].variables.SITE_COGNITO_REDIRECT_URI == "https://dev.craigdevjohnson.com/auth/callback" &&
      aws_lambda_function.app.environment[0].variables.SITE_COGNITO_LOGOUT_URI == "https://dev.craigdevjohnson.com/sign-in" &&
      jsondecode(aws_lambda_function.app.environment[0].variables.SITE_INVITATIONS_JSON)["craigdevjohnson@gmail.com"] == ["management", "soccer"] &&
      aws_lambda_function.app.environment[0].variables.SITE_ALLOW_LOCAL_CALLBACK == "false"
    )
    error_message = "reviewed development site identity must wire only its own runtime and session path"
  }
}

run "site_rejects_wrong_environment_callback" {
  command = plan

  variables {
    site = {
      cognito_domain       = "https://portfolio-lambda-dev-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_DevSite"
      cognito_client_id    = "devsiteclient"
      redirect_uri         = "https://craigdevjohnson.com/auth/callback"
      logout_uri           = "https://dev.craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer", "management"] }
      allow_local_callback = false
    }
  }

  expect_failures = [var.site]
}

run "production_site_runtime_contract" {
  command = plan

  variables {
    environment = "prod"
    name_prefix = "portfolio-lambda-prod"
    site = {
      cognito_domain       = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_ProdSite"
      cognito_client_id    = "prodsiteclient"
      redirect_uri         = "https://craigdevjohnson.com/auth/callback"
      logout_uri           = "https://craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer", "management"] }
      allow_local_callback = false
    }
  }

  assert {
    condition = aws_lambda_function.app.environment[0].variables == tomap({
      CLIENT_ID_KEY                = "/portfolio/lambda/prod/CLIENT_ID_KEY"
      CLIENT_SECRET_KEY            = "/portfolio/lambda/prod/CLIENT_SECRET_KEY"
      GOOGLE_CONNECTION_TABLE_NAME = "portfolio-lambda-prod-google-connections"
      LOG_ADD_SOURCE               = "false"
      LOG_FORMAT                   = "json"
      LOG_LEVEL                    = "info"
      LPS_SESSION_KEY              = "/portfolio/lambda/prod/LPS_SESSION_KEY"
      SOCCER_SESSION_TABLE_NAME    = "portfolio-lambda-prod-soccer-sessions"
      SITE_SESSION_KEY             = "/portfolio/lambda/prod/SITE_SESSION_KEY"
      SITE_COGNITO_DOMAIN          = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
      SITE_COGNITO_ISSUER          = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_ProdSite"
      SITE_COGNITO_CLIENT_ID       = "prodsiteclient"
      SITE_COGNITO_REDIRECT_URI    = "https://craigdevjohnson.com/auth/callback"
      SITE_COGNITO_LOGOUT_URI      = "https://craigdevjohnson.com/sign-in"
      SITE_INVITATIONS_JSON        = "{\"craigdevjohnson@gmail.com\":[\"management\",\"soccer\"]}"
      SITE_ALLOW_LOCAL_CALLBACK    = "false"
    })
    error_message = "production site identity must reach Lambda as its own public settings and a session parameter path, without management settings"
  }

  assert {
    condition = (
      output.ssm_parameter_paths == tomap({
        CLIENT_ID_KEY     = "/portfolio/lambda/prod/CLIENT_ID_KEY"
        CLIENT_SECRET_KEY = "/portfolio/lambda/prod/CLIENT_SECRET_KEY"
        LPS_SESSION_KEY   = "/portfolio/lambda/prod/LPS_SESSION_KEY"
        SITE_SESSION_KEY  = "/portfolio/lambda/prod/SITE_SESSION_KEY"
      }) &&
      length([for st in data.aws_iam_policy_document.lambda.statement : st if
        st.actions == toset(["kms:Decrypt"]) && length(st.condition) == 1 &&
        alltrue([for c in st.condition : c.test == "StringEquals" && c.variable == "kms:EncryptionContext:PARAMETER_ARN" &&
          toset(c.values) == toset([
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/prod/CLIENT_ID_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/prod/CLIENT_SECRET_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/prod/LPS_SESSION_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/prod/SITE_SESSION_KEY",
      ])])]) == 1
    )
    error_message = "production may read and decrypt only its own four SecureStrings once site identity is enabled"
  }
}

run "site_rejects_development_identity_in_production" {
  command = plan

  variables {
    environment = "prod"
    name_prefix = "portfolio-lambda-prod"
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

  expect_failures = [var.site]
}

run "site_rejects_production_loopback_callback" {
  command = plan

  variables {
    environment = "prod"
    name_prefix = "portfolio-lambda-prod"
    site = {
      cognito_domain       = "https://portfolio-lambda-prod-site-793680745829.auth.us-west-2.amazoncognito.com"
      cognito_issuer       = "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_ProdSite"
      cognito_client_id    = "prodsiteclient"
      redirect_uri         = "https://craigdevjohnson.com/auth/callback"
      logout_uri           = "https://craigdevjohnson.com/sign-in"
      invitations          = { "craigdevjohnson@gmail.com" = ["soccer", "management"] }
      allow_local_callback = true
    }
  }

  expect_failures = [var.site]
}

run "history_disabled_without_reviewed_limits" {
  command = plan

  assert {
    condition = (
      length(aws_lambda_function.history_worker) == 0 &&
      length(aws_scheduler_schedule.history_daily) == 0 &&
      length(aws_sqs_queue.history_dead_letter) == 0 &&
      length(aws_cloudwatch_metric_alarm.history_admission_rejected) == 0 &&
      !contains(keys(aws_lambda_function.app.environment[0].variables), "SOCCER_HISTORY_COLLECTION_ENABLED")
    )
    error_message = "unset reviewed limits must leave collection and daily scheduling disabled"
  }
}

run "history_schedule_rejects_missing_limits" {
  command = plan
  variables {
    enable_soccer_history              = true
    activate_soccer_history_collection = true
    activate_soccer_history_schedule   = true
    soccer_history_schedule_expression = "cron(0 12 * * ? *)"
  }
  expect_failures = [aws_iam_role.lambda]
}

run "history_collection_rejects_missing_table" {
  command = plan
  variables {
    activate_soccer_history_collection = true
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  expect_failures = [aws_iam_role.lambda]
}

run "history_collection_rejects_missing_alert_destination" {
  command = plan
  variables {
    enable_soccer_history              = true
    alarm_action_arns                  = []
    activate_soccer_history_collection = true
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  expect_failures = [aws_iam_role.lambda]
}

run "history_schedule_rejects_missing_expression" {
  command = plan
  variables {
    enable_soccer_history              = true
    alarm_action_arns                  = ["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"]
    activate_soccer_history_collection = true
    activate_soccer_history_schedule   = true
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  expect_failures = [aws_iam_role.lambda]
}

run "history_limits_reject_a_timeout_covering_only_pacing" {
  command = plan
  variables {
    enable_soccer_history = true
    # 99 paced gaps of 2 seconds fit 200 seconds, but one slow team with five
    # retries and the worker's wrap-up time does not.
    soccer_history_limits = {
      max_enrolled_teams      = 100
      reserved_player_slots   = 10
      max_requests_per_run    = 100
      max_retries_per_team    = 5
      min_request_interval_ms = 2000
      worker_timeout_seconds  = 200
    }
  }
  expect_failures = [var.soccer_history_limits]
}

run "history_limits_alone_do_not_activate" {
  command = plan
  variables {
    enable_soccer_history = true
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  assert {
    condition = (
      length(aws_lambda_function.history_worker) == 0 &&
      length(aws_scheduler_schedule.history_daily) == 0 &&
      !contains(keys(aws_lambda_function.app.environment[0].variables), "SOCCER_HISTORY_COLLECTION_ENABLED")
    )
    error_message = "numeric limits alone must not activate collection or polling"
  }
}

run "history_collection_has_capacity_alert_without_schedule" {
  command = plan
  variables {
    enable_soccer_history              = true
    alarm_action_arns                  = ["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"]
    activate_soccer_history_collection = true
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  assert {
    condition = (
      aws_lambda_function.app.environment[0].variables.SOCCER_HISTORY_COLLECTION_ENABLED == "true" &&
      aws_lambda_function.app.environment[0].variables.SOCCER_HISTORY_MAX_TEAMS == "4" &&
      aws_lambda_function.app.environment[0].variables.SOCCER_ARCHIVE_TABLE_NAME == "portfolio-lambda-dev-soccer-history" &&
      length([
        for statement in data.aws_iam_policy_document.lambda.statement : statement
        if toset(statement.actions) == toset(["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query"]) &&
        toset(statement.resources) == toset([aws_dynamodb_table.soccer_history[0].arn])
      ]) == 1 &&
      length(aws_cloudwatch_log_metric_filter.history_admission_rejected) == 1 &&
      aws_cloudwatch_log_metric_filter.history_admission_rejected[0].pattern == "{ $.msg = \"soccer_history_admission_rejected\" }" &&
      length(aws_cloudwatch_metric_alarm.history_admission_rejected) == 1 &&
      length(aws_lambda_function.history_worker) == 0 &&
      length(aws_scheduler_schedule.history_daily) == 0
    )
    error_message = "collection must have a capacity alarm without implicitly starting a schedule"
  }
}

run "history_worker_schedule_and_failure_contract" {
  command = plan
  variables {
    enable_soccer_history              = true
    alarm_action_arns                  = ["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"]
    activate_soccer_history_collection = true
    activate_soccer_history_schedule   = true
    soccer_history_schedule_expression = "cron(0 12 * * ? *)"
    soccer_history_limits = {
      max_enrolled_teams      = 4
      reserved_player_slots   = 2
      max_requests_per_run    = 8
      max_retries_per_team    = 1
      min_request_interval_ms = 250
      worker_timeout_seconds  = 120
    }
  }
  assert {
    condition = (
      length(aws_lambda_function.history_worker) == 1 &&
      aws_lambda_function.history_worker[0].reserved_concurrent_executions == 1 &&
      aws_lambda_function.history_worker[0].timeout == 120 &&
      aws_lambda_function.history_worker[0].environment[0].variables.SOCCER_HISTORY_MODE == "scheduled" &&
      aws_lambda_function.history_worker[0].environment[0].variables.SOCCER_HISTORY_MAX_REQUESTS == "8" &&
      aws_scheduler_schedule.history_daily[0].schedule_expression == "cron(0 12 * * ? *)" &&
      aws_scheduler_schedule.history_daily[0].target[0].arn == aws_lambda_function.history_worker[0].arn &&
      aws_scheduler_schedule.history_daily[0].target[0].dead_letter_config[0].arn == aws_sqs_queue.history_dead_letter[0].arn &&
      aws_lambda_function_event_invoke_config.history_worker[0].destination_config[0].on_failure[0].destination == aws_sqs_queue.history_dead_letter[0].arn &&
      length(aws_cloudwatch_metric_alarm.history_worker_errors) == 1 &&
      length(aws_cloudwatch_metric_alarm.history_incomplete) == 1 &&
      length(aws_cloudwatch_metric_alarm.history_dead_letter) == 1 &&
      aws_cloudwatch_log_metric_filter.history_incomplete[0].pattern == "{ $.msg = \"soccer_history_daily_incomplete\" }" &&
      length(output.alarm_arns) == 9
    )
    error_message = "the daily worker must be bounded, separately invoked, and monitored for delivery, execution, and partial failure"
  }
  assert {
    condition = (
      length(data.aws_iam_policy_document.history_worker[0].statement) == 4 &&
      length([for st in data.aws_iam_policy_document.history_worker[0].statement : st if
        toset(st.actions) == toset(["dynamodb:GetItem", "dynamodb:PutItem"]) &&
      toset(st.resources) == toset([aws_dynamodb_table.soccer_history[0].arn])]) == 1 &&
      length([for st in data.aws_iam_policy_document.history_worker[0].statement : st if
        toset(st.actions) == toset(["dynamodb:Query"]) &&
      toset(st.resources) == toset(["${aws_dynamodb_table.soccer_history[0].arn}/index/due-teams"])]) == 1 &&
      length(data.aws_iam_policy_document.history_scheduler[0].statement) == 2
    )
    error_message = "worker and Scheduler roles must have only scoped table, due-index, invoke, DLQ, and log rights"
  }
  assert {
    condition = (
      aws_iam_role.history_worker[0].permissions_boundary == "arn:aws:iam::111122223333:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
      aws_iam_role.history_scheduler[0].permissions_boundary == "arn:aws:iam::111122223333:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" &&
      length([for st in data.aws_iam_policy_document.history_scheduler[0].statement : st if
        toset(st.actions) == toset(["lambda:InvokeFunction"]) &&
      toset(st.resources) == toset([aws_lambda_function.history_worker[0].arn])]) == 1 &&
      length([for st in data.aws_iam_policy_document.history_scheduler[0].statement : st if
        toset(st.actions) == toset(["sqs:SendMessage"]) &&
      toset(st.resources) == toset([aws_sqs_queue.history_dead_letter[0].arn])]) == 1 &&
      length([for st in data.aws_iam_policy_document.history_worker[0].statement : st if
        toset(st.actions) == toset(["logs:CreateLogStream", "logs:PutLogEvents"]) &&
      toset(st.resources) == toset(["${aws_cloudwatch_log_group.history_worker[0].arn}:*"])]) == 1 &&
      length([for st in data.aws_iam_policy_document.history_worker[0].statement : st if
        toset(st.actions) == toset(["sqs:SendMessage"]) &&
      toset(st.resources) == toset([aws_sqs_queue.history_dead_letter[0].arn])]) == 1 &&
      one(data.aws_iam_policy_document.history_scheduler_assume[0].statement[0].condition).values == tolist(["arn:aws:scheduler:us-west-2:111122223333:schedule/default/portfolio-lambda-dev-soccer-history-daily"])
    )
    error_message = "the worker and Scheduler roles must sit inside the execution boundary and reach only the worker, its log group, and the failure queue"
  }
  assert {
    condition = (
      aws_scheduler_schedule.history_daily[0].schedule_expression_timezone == "UTC" &&
      aws_scheduler_schedule.history_daily[0].state == "ENABLED" &&
      aws_scheduler_schedule.history_daily[0].flexible_time_window[0].mode == "OFF" &&
      aws_scheduler_schedule.history_daily[0].target[0].retry_policy[0].maximum_retry_attempts == 2 &&
      aws_lambda_function_event_invoke_config.history_worker[0].maximum_retry_attempts == 0 &&
      aws_cloudwatch_log_metric_filter.history_incomplete[0].log_group_name == "/aws/lambda/portfolio-lambda-dev-soccer-history" &&
      aws_cloudwatch_log_metric_filter.history_admission_rejected[0].log_group_name == "/aws/lambda/portfolio-lambda-dev" &&
      aws_cloudwatch_log_metric_filter.history_admission_rejected[0].pattern == "{ $.msg = \"soccer_history_admission_rejected\" }" &&
      aws_cloudwatch_metric_alarm.history_worker_errors[0].dimensions == tomap({ FunctionName = "portfolio-lambda-dev-soccer-history" }) &&
      alltrue([
        for alarm in [
          aws_cloudwatch_metric_alarm.history_admission_rejected[0],
          aws_cloudwatch_metric_alarm.history_incomplete[0],
          aws_cloudwatch_metric_alarm.history_worker_errors[0],
          aws_cloudwatch_metric_alarm.history_dead_letter[0],
        ] : alarm.threshold == 1 && toset(alarm.alarm_actions) == toset(["arn:aws:sns:us-west-2:111122223333:portfolio-lambda-alerts"])
      ])
    )
    error_message = "the daily schedule must fire at a fixed UTC time, retry delivery without re-running work, and alert on every rejected enrollment, incomplete run, worker error, and failed delivery"
  }
}
