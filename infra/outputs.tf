output "ecr_repository_url" {
  description = "ECR repository URL — use this to tag and push your Docker image"
  value       = aws_ecr_repository.app.repository_url
}

output "app_runner_service_url" {
  description = "Public URL of the App Runner service"
  value       = "https://${aws_apprunner_service.app.service_url}"
}

output "app_runner_service_arn" {
  description = "ARN of the App Runner service"
  value       = aws_apprunner_service.app.arn
}

output "app_runner_service_id" {
  description = "ID of the App Runner service"
  value       = aws_apprunner_service.app.service_id
}

output "google_connection_table_arn" {
  description = "ARN of the DynamoDB table used for persistent Google Calendar connections"
  value       = aws_dynamodb_table.google_connections.arn
}

output "google_connection_table_name" {
  description = "Name of the DynamoDB table used for persistent Google Calendar connections"
  value       = aws_dynamodb_table.google_connections.name
}

output "instance_role_arn" {
  description = "ARN of the App Runner instance role (attach future policies here)"
  value       = aws_iam_role.apprunner_instance.arn
}

output "lambda_function_name" {
  description = "Name of the Lambda function backing the portfolio app"
  value       = aws_lambda_function.app.function_name
}

output "lambda_api_url" {
  description = "Invoke URL for the API Gateway HTTP API fronting Lambda"
  value       = aws_apigatewayv2_api.lambda.api_endpoint
}
