output "ecr_repository_name" {
  value = aws_ecr_repository.lambda_releases.name
}

output "ecr_repository_arn" {
  value = aws_ecr_repository.lambda_releases.arn
}

output "ecr_repository_url" {
  value = aws_ecr_repository.lambda_releases.repository_url
}
