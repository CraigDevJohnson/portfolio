output "role_arns" {
  description = "Role ARNs for the AWS_*_ROLE_ARN GitHub variables."
  value = {
    AWS_RELEASE_BUILDER_ROLE_ARN      = aws_iam_role.ci["release"].arn
    AWS_DEVELOPMENT_DEPLOYER_ROLE_ARN = aws_iam_role.ci["dev"].arn
    AWS_PRODUCTION_PLANNER_ROLE_ARN   = aws_iam_role.ci["prod"].arn
    AWS_PRODUCTION_DEPLOYER_ROLE_ARN  = aws_iam_role.production_deployer.arn
  }
}

output "lambda_execution_boundary_arn" {
  value = aws_iam_policy.lambda_execution_boundary.arn
}

output "lambda_history_execution_boundary_arn" {
  value = aws_iam_policy.lambda_history_execution_boundary.arn
}

output "state_bucket_name" {
  value = aws_s3_bucket.state.bucket
}
