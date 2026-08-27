resource "aws_lambda_function" "app" {
  function_name                  = local.function_name
  role                           = aws_iam_role.lambda.arn
  package_type                   = "Image"
  architectures                  = ["x86_64"]
  image_uri                      = local.image_uri
  memory_size                    = var.lambda_memory_mb
  timeout                        = var.lambda_timeout_seconds
  reserved_concurrent_executions = var.reserved_concurrency
  publish                        = true

  environment {
    variables = {
      CLIENT_ID_KEY                = local.ssm_paths.CLIENT_ID_KEY
      CLIENT_SECRET_KEY            = local.ssm_paths.CLIENT_SECRET_KEY
      GOOGLE_CONNECTION_TABLE_NAME = aws_dynamodb_table.google_connections.name
      LOG_ADD_SOURCE               = "false"
      LOG_FORMAT                   = "json"
      LOG_LEVEL                    = "info"
      LPS_SESSION_KEY              = local.ssm_paths.LPS_SESSION_KEY
      SOCCER_SESSION_TABLE_NAME    = aws_dynamodb_table.soccer_sessions.name
    }
  }

  depends_on = [aws_cloudwatch_log_group.lambda, aws_iam_role_policy.lambda]
}

locals {
  live_version = var.live_version_override == null ? aws_lambda_function.app.version : tostring(var.live_version_override)
}

resource "aws_lambda_alias" "live" {
  name             = "live"
  function_name    = aws_lambda_function.app.function_name
  function_version = local.live_version
}
