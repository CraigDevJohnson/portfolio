terraform {
  required_version = "= 1.12.6"
  required_providers {
    aws = { source = "hashicorp/aws", version = "= 6.38.0" }
  }
}

provider "aws" { region = "us-west-2" }

data "aws_iam_policy_document" "release_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals { type = "Federated"
      identifiers = ["arn:aws:iam::180294223248:oidc-provider/token.actions.githubusercontent.com"] }
    condition { test = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values = ["sts.amazonaws.com"] }
    condition { test = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values = ["repo:CraigDevJohnson/portfolio:ref:refs/heads/main"] }
  }
}

data "aws_iam_policy_document" "environment_trust" {
  for_each = toset(["development", "production"])
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals { type = "Federated"
      identifiers = ["arn:aws:iam::180294223248:oidc-provider/token.actions.githubusercontent.com"] }
    condition { test = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values = ["sts.amazonaws.com"] }
    condition { test = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values = ["repo:CraigDevJohnson/portfolio:environment:${each.key}"] }
  }
}

locals {
  roles = {
    release = { name = "portfolio-release-builder-ci", trust = data.aws_iam_policy_document.release_trust.json }
    dev     = { name = "portfolio-development-deployer-ci", trust = data.aws_iam_policy_document.environment_trust["development"].json }
    prod    = { name = "portfolio-production-deployer-ci", trust = data.aws_iam_policy_document.environment_trust["production"].json }
  }
}

resource "aws_iam_role" "ci" {
  for_each           = local.roles
  name               = each.value.name
  assume_role_policy = each.value.trust
  max_session_duration = 3600
}

resource "aws_iam_role_policy" "release" {
  name = "portfolio-release-builder"
  role = aws_iam_role.ci["release"].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["ecr:GetAuthorizationToken"], Resource = "*" },
    { Effect = "Allow", Action = ["ecr:BatchCheckLayerAvailability", "ecr:CompleteLayerUpload", "ecr:DescribeImages", "ecr:DescribeImageScanFindings", "ecr:DescribeRepositories", "ecr:GetDownloadUrlForLayer", "ecr:InitiateLayerUpload", "ecr:ListImages", "ecr:PutImage", "ecr:UploadLayerPart"], Resource = "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases" }
  ] })
}

locals {
  state_bucket = "arn:aws:s3:::portfolio-tofu-state-180294223248"
  environment_actions = [
    "apigateway:GET", "apigateway:PATCH", "apigateway:POST", "apigateway:PUT",
    "cloudwatch:DescribeAlarms", "cloudwatch:GetMetricData", "cloudwatch:ListTagsForResource", "cloudwatch:PutMetricAlarm", "cloudwatch:TagResource",
    "dynamodb:CreateTable", "dynamodb:DescribeContinuousBackups", "dynamodb:DescribeTable", "dynamodb:DescribeTimeToLive", "dynamodb:ListTagsOfResource", "dynamodb:TagResource", "dynamodb:UpdateContinuousBackups", "dynamodb:UpdateTable",
    "iam:CreateRole", "iam:GetRole", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies", "iam:ListRolePolicies", "iam:ListRoleTags", "iam:PassRole", "iam:PutRolePolicy", "iam:TagRole",
    "lambda:AddPermission", "lambda:CreateAlias", "lambda:CreateFunction", "lambda:GetAlias", "lambda:GetFunction", "lambda:GetFunctionConfiguration", "lambda:GetPolicy", "lambda:ListTags", "lambda:ListVersionsByFunction", "lambda:PublishVersion", "lambda:TagResource", "lambda:UpdateAlias", "lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
    "logs:CreateLogGroup", "logs:DescribeLogGroups", "logs:ListTagsForResource", "logs:PutRetentionPolicy", "logs:TagResource",
    "ssm:GetParameter", "ssm:GetParameters"
  ]
}

resource "aws_iam_role_policy" "environment" {
  for_each = { dev = "dev", prod = "prod" }
  name = "portfolio-${each.value}-replacement-root"
  role = aws_iam_role.ci[each.key].id
  policy = jsonencode({ Version = "2012-10-17", Statement = [
    { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"], Resource = "${local.state_bucket}/portfolio-lambda-http-api/${each.value}/terraform.tfstate*" },
    { Effect = "Allow", Action = ["s3:ListBucket"], Resource = local.state_bucket, Condition = { StringLike = { "s3:prefix" = ["portfolio-lambda-http-api/${each.value}/terraform.tfstate*"] } } },
    { Effect = "Allow", Action = local.environment_actions, Resource = "*", Condition = { StringEqualsIfExists = { "aws:ResourceTag/Project" = "portfolio", "aws:ResourceTag/Environment" = each.value } } },
    { Effect = "Allow", Action = ["ecr:BatchGetImage", "ecr:DescribeImages", "ecr:GetDownloadUrlForLayer"], Resource = "arn:aws:ecr:us-west-2:180294223248:repository/portfolio-lambda-releases" }
  ] })
}

output "role_arns" { value = { for key, role in aws_iam_role.ci : key => role.arn } }
