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

output "instance_role_arn" {
  description = "ARN of the App Runner instance role (attach future policies here)"
  value       = aws_iam_role.apprunner_instance.arn
}
