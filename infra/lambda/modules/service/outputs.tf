output "environment" {
  value = var.environment
}

output "image_uri" {
  value = aws_lambda_function.app.image_uri
}

output "lambda_function_name" {
  value = aws_lambda_function.app.function_name
}

output "lambda_function_arn" {
  value = aws_lambda_function.app.arn
}

output "lambda_published_version" {
  value = aws_lambda_function.app.version
}

output "lambda_alias_name" {
  value = aws_lambda_alias.live.name
}

output "lambda_alias_arn" {
  value = aws_lambda_alias.live.arn
}

output "lambda_execution_role_name" {
  value = aws_iam_role.lambda.name
}

output "lambda_execution_permissions_boundary_arn" {
  value = aws_iam_role.lambda.permissions_boundary
}

output "lambda_runtime_policy_name" {
  value = aws_iam_role_policy.lambda.name
}

output "api_id" {
  value = aws_apigatewayv2_api.app.id
}

output "api_default_url" {
  value = aws_apigatewayv2_api.app.api_endpoint
}

output "api_name" {
  value = aws_apigatewayv2_api.app.name
}

output "lambda_log_group_name" {
  value = aws_cloudwatch_log_group.lambda.name
}

output "api_access_log_group_name" {
  value = aws_cloudwatch_log_group.api_access.name
}

output "google_connection_table_name" {
  value = aws_dynamodb_table.google_connections.name
}

output "google_connection_table_arn" {
  value = aws_dynamodb_table.google_connections.arn
}

output "soccer_session_table_name" {
  value = aws_dynamodb_table.soccer_sessions.name
}

output "soccer_session_table_arn" {
  value = aws_dynamodb_table.soccer_sessions.arn
}

output "ssm_parameter_paths" {
  value = tomap(local.ssm_paths)
}

output "alarm_arns" {
  value = sort([
    aws_cloudwatch_metric_alarm.api_5xx.arn,
    aws_cloudwatch_metric_alarm.api_latency.arn,
    aws_cloudwatch_metric_alarm.lambda_duration.arn,
    aws_cloudwatch_metric_alarm.lambda_errors.arn,
    aws_cloudwatch_metric_alarm.lambda_throttles.arn,
  ])
}

output "alarm_names" {
  value = sort([
    aws_cloudwatch_metric_alarm.api_5xx.alarm_name,
    aws_cloudwatch_metric_alarm.api_latency.alarm_name,
    aws_cloudwatch_metric_alarm.lambda_duration.alarm_name,
    aws_cloudwatch_metric_alarm.lambda_errors.alarm_name,
    aws_cloudwatch_metric_alarm.lambda_throttles.alarm_name,
  ])
}

output "certificate_arn" {
  value = var.request_custom_domain ? aws_acm_certificate.custom[0].arn : tostring(null)
}

output "acm_validation_records" {
  value = local.acm_validation_records
}

output "api_gateway_domain_targets" {
  value = local.api_gateway_domain_targets
}

output "oauth_redirect_uris" {
  value = sort([for domain_name in var.domain_names : "https://${domain_name}/soccer"])
}
