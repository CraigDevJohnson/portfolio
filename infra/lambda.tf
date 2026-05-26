# ──────────────────────────────────────────────
# IAM — Lambda execution role
# ──────────────────────────────────────────────

resource "aws_iam_role" "lambda_execution" {
  name = "${var.app_name}-lambda-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_basic_execution" {
  role       = aws_iam_role.lambda_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy_attachment" "lambda_google_connections_dynamodb" {
  role       = aws_iam_role.lambda_execution.name
  policy_arn = aws_iam_policy.google_connections_dynamodb.arn
}

resource "aws_iam_role_policy_attachment" "lambda_soccer_sessions_dynamodb" {
  role       = aws_iam_role.lambda_execution.name
  policy_arn = aws_iam_policy.soccer_sessions_dynamodb.arn
}

data "aws_ssm_parameter" "lambda_runtime_secrets" {
  for_each        = toset(local.ssm_parameter_names)
  name            = "/${var.app_name}/${each.key}"
  with_decryption = true
}

# ──────────────────────────────────────────────
# Lambda function (container image in ECR)
# ──────────────────────────────────────────────

resource "aws_lambda_function" "app" {
  function_name = "${var.app_name}-lambda"
  role          = aws_iam_role.lambda_execution.arn
  package_type  = "Image"
  image_uri     = "${aws_ecr_repository.app.repository_url}:${var.lambda_image_tag}"
  timeout       = var.lambda_timeout_seconds
  memory_size   = var.lambda_memory_mb

  environment {
    variables = {
      APP_BIND_ALL                 = "true"
      CLIENT_ID_KEY                = data.aws_ssm_parameter.lambda_runtime_secrets["CLIENT_ID_KEY"].value
      CLIENT_SECRET_KEY            = data.aws_ssm_parameter.lambda_runtime_secrets["CLIENT_SECRET_KEY"].value
      GOOGLE_CONNECTION_TABLE_NAME = local.google_connection_table_name
      LPS_SESSION_KEY              = data.aws_ssm_parameter.lambda_runtime_secrets["LPS_SESSION_KEY"].value
      LOG_ADD_SOURCE               = "false"
      LOG_FORMAT                   = "json"
      LOG_LEVEL                    = "info"
      SOCCER_SESSION_TABLE_NAME    = local.soccer_session_table_name
    }
  }

  depends_on = [
    aws_iam_role_policy_attachment.lambda_basic_execution,
    aws_iam_role_policy_attachment.lambda_google_connections_dynamodb,
    aws_iam_role_policy_attachment.lambda_soccer_sessions_dynamodb,
  ]
}

# ──────────────────────────────────────────────
# API Gateway HTTP API -> Lambda proxy integration
# ──────────────────────────────────────────────

resource "aws_apigatewayv2_api" "lambda" {
  name          = "${var.app_name}-lambda-http"
  protocol_type = "HTTP"
}

resource "aws_apigatewayv2_integration" "lambda_proxy" {
  api_id                 = aws_apigatewayv2_api.lambda.id
  integration_type       = "AWS_PROXY"
  integration_uri        = aws_lambda_function.app.invoke_arn
  integration_method     = "POST"
  payload_format_version = "2.0"
}

resource "aws_apigatewayv2_route" "lambda_default" {
  api_id    = aws_apigatewayv2_api.lambda.id
  route_key = "$default"
  target    = "integrations/${aws_apigatewayv2_integration.lambda_proxy.id}"
}

resource "aws_apigatewayv2_stage" "lambda_default" {
  api_id      = aws_apigatewayv2_api.lambda.id
  name        = "$default"
  auto_deploy = true
}

resource "aws_lambda_permission" "allow_apigateway_invoke" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.app.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.lambda.execution_arn}/*/*"
}
