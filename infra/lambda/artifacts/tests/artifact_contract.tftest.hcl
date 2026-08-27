mock_provider "aws" {
  mock_resource "aws_ecr_repository" {
    defaults = {
      arn            = "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases"
      repository_url = "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases"
    }
  }
}

run "artifact_ownership_contract" {
  command = plan

  assert {
    condition = (
      aws_ecr_repository_policy.lambda_releases.repository == aws_ecr_repository.lambda_releases.name &&
      jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Version == "2012-10-17" &&
      length(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement) == 1 &&
      jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Effect == "Allow" &&
      jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Principal == {
        Service = "lambda.amazonaws.com"
      } &&
      length(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Action) == 2 &&
      toset(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Action) == toset([
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer",
      ])
    )
    error_message = "the artifact root must own the exact Lambda-only ECR pull policy"
  }

  assert {
    condition = (
      toset(keys(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Condition)) == toset(["ArnLike", "StringEquals"]) &&
      toset(keys(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Condition.StringEquals)) == toset(["aws:SourceAccount"]) &&
      jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Condition.StringEquals["aws:SourceAccount"] == "180294223248" &&
      toset(keys(jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Condition.ArnLike)) == toset(["aws:SourceArn"]) &&
      jsondecode(aws_ecr_repository_policy.lambda_releases.policy).Statement[0].Condition.ArnLike["aws:SourceArn"] == "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"
    )
    error_message = "the ECR pull policy must retain the reviewed source account and Lambda ARN conditions"
  }
}
