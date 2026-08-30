terraform {
  required_version = "= 1.12.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "= 6.38.0"
    }
  }

  backend "s3" {}
}

provider "aws" {
  region              = local.region
  profile             = "portfolio-ci-roles-administrator"
  allowed_account_ids = ["180294223248"]
}

data "aws_iam_policy_document" "release_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.github_oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:CraigDevJohnson/portfolio:ref:refs/heads/main"]
    }
  }
}

data "aws_iam_policy_document" "environment_trust" {
  for_each = local.environment_configuration

  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [local.github_oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:CraigDevJohnson/portfolio:environment:${each.value.github_environment}"]
    }
  }
}

locals {
  account_id               = "180294223248"
  region                   = "us-west-2"
  state_bucket_name        = "portfolio-tofu-state-${local.account_id}"
  state_bucket_arn         = "arn:aws:s3:::${local.state_bucket_name}"
  ecr_repository_arn       = "arn:aws:ecr:${local.region}:${local.account_id}:repository/portfolio-lambda-releases"
  github_oidc_provider_arn = "arn:aws:iam::${local.account_id}:oidc-provider/token.actions.githubusercontent.com"

  required_tags = {
    ManagedBy = "opentofu"
    Platform  = "lambda-http-api"
    Project   = "portfolio"
  }

  environment_configuration = {
    dev = {
      environment        = "dev"
      github_environment = "development"
      role_name          = "portfolio-development-deployer-ci"
      policy_name        = "portfolio-development-runtime-release"
      state_key          = "portfolio-lambda-http-api/dev/terraform.tfstate"
      function_name      = "portfolio-lambda-dev"
      mutable            = true
    }
    prod = {
      environment        = "prod"
      github_environment = "production-plan"
      role_name          = "portfolio-production-planner-ci"
      policy_name        = "portfolio-production-read-only-plan"
      state_key          = "portfolio-lambda-http-api/prod/terraform.tfstate"
      function_name      = "portfolio-lambda-prod"
      mutable            = false
    }
  }

  roles = merge(
    {
      release = {
        name  = "portfolio-release-builder-ci"
        trust = data.aws_iam_policy_document.release_trust.json
      }
    },
    {
      for key, configuration in local.environment_configuration : key => {
        name  = configuration.role_name
        trust = data.aws_iam_policy_document.environment_trust[key].json
      }
    },
  )

  environment_read_statements = {
    for key, configuration in local.environment_configuration : key => [
      {
        Sid      = "CallerIdentity"
        Effect   = "Allow"
        Action   = ["sts:GetCallerIdentity"]
        Resource = "*"
      },
      {
        Sid      = "StateBucketMetadata"
        Effect   = "Allow"
        Action   = ["s3:GetBucketLocation", "s3:GetBucketVersioning"]
        Resource = local.state_bucket_arn
      },
      {
        Sid      = "StatePrefix"
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = local.state_bucket_arn
        Condition = {
          StringLike = {
            "s3:prefix" = ["${configuration.state_key}*"]
          }
        }
      },
      {
        Sid      = "StateRead"
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = "${local.state_bucket_arn}/${configuration.state_key}"
      },
      {
        Sid      = "StateLock"
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = "${local.state_bucket_arn}/${configuration.state_key}.tflock"
      },
      {
        Sid      = "ReleaseImageRead"
        Effect   = "Allow"
        Action   = ["ecr:BatchGetImage", "ecr:DescribeImages", "ecr:GetDownloadUrlForLayer"]
        Resource = local.ecr_repository_arn
      },
      {
        Sid    = "ExecutionRoleRead"
        Effect = "Allow"
        Action = [
          "iam:GetRole",
          "iam:GetRolePolicy",
          "iam:ListAttachedRolePolicies",
          "iam:ListRolePolicies",
          "iam:ListRoleTags",
        ]
        Resource = "arn:aws:iam::${local.account_id}:role/${configuration.function_name}-execution"
      },
      {
        Sid    = "LambdaRead"
        Effect = "Allow"
        Action = [
          "lambda:GetAlias",
          "lambda:GetFunction",
          "lambda:GetFunctionCodeSigningConfig",
          "lambda:GetFunctionConcurrency",
          "lambda:GetFunctionConfiguration",
          "lambda:GetPolicy",
          "lambda:GetRuntimeManagementConfig",
          "lambda:ListTags",
          "lambda:ListVersionsByFunction",
        ]
        Resource = [
          "arn:aws:lambda:${local.region}:${local.account_id}:function:${configuration.function_name}",
          "arn:aws:lambda:${local.region}:${local.account_id}:function:${configuration.function_name}:*",
        ]
      },
      {
        Sid      = "ApiGatewayRead"
        Effect   = "Allow"
        Action   = ["apigateway:GET"]
        Resource = ["arn:aws:apigateway:${local.region}::/apis*", "arn:aws:apigateway:${local.region}::/domainnames*"]
      },
      {
        Sid    = "LogGroupRead"
        Effect = "Allow"
        Action = ["logs:ListTagsForResource"]
        Resource = [
          "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/apigateway/${configuration.function_name}/access",
          "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/apigateway/${configuration.function_name}/access:*",
          "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/lambda/${configuration.function_name}",
          "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/lambda/${configuration.function_name}:*",
        ]
      },
      {
        Sid      = "LogGroupList"
        Effect   = "Allow"
        Action   = ["logs:DescribeLogGroups"]
        Resource = "*"
        Condition = {
          StringEquals = {
            "aws:RequestedRegion" = local.region
          }
        }
      },
      {
        Sid    = "TableRead"
        Effect = "Allow"
        Action = [
          "dynamodb:DescribeContinuousBackups",
          "dynamodb:DescribeTable",
          "dynamodb:DescribeTimeToLive",
          "dynamodb:ListTagsOfResource",
        ]
        Resource = [
          "arn:aws:dynamodb:${local.region}:${local.account_id}:table/${configuration.function_name}-google-connections",
          "arn:aws:dynamodb:${local.region}:${local.account_id}:table/${configuration.function_name}-soccer-sessions",
        ]
      },
      {
        Sid      = "KmsAliasList"
        Effect   = "Allow"
        Action   = ["kms:ListAliases"]
        Resource = "*"
        Condition = {
          StringEquals = {
            "aws:RequestedRegion" = local.region
          }
        }
      },
      {
        Sid      = "KmsSsmKeyRead"
        Effect   = "Allow"
        Action   = ["kms:DescribeKey"]
        Resource = "arn:aws:kms:${local.region}:${local.account_id}:key/*"
        Condition = {
          "ForAnyValue:StringEquals" = {
            "kms:ResourceAliases" = "alias/aws/ssm"
          }
        }
      },
      {
        Sid      = "CertificateRead"
        Effect   = "Allow"
        Action   = ["acm:DescribeCertificate", "acm:ListTagsForCertificate"]
        Resource = "arn:aws:acm:${local.region}:${local.account_id}:certificate/*"
        Condition = {
          StringEquals = {
            "aws:RequestedRegion"         = local.region
            "aws:ResourceTag/Environment" = configuration.environment
            "aws:ResourceTag/ManagedBy"   = local.required_tags.ManagedBy
            "aws:ResourceTag/Platform"    = local.required_tags.Platform
            "aws:ResourceTag/Project"     = local.required_tags.Project
          }
        }
      },
      {
        Sid    = "AlarmRead"
        Effect = "Allow"
        Action = ["cloudwatch:DescribeAlarms", "cloudwatch:ListTagsForResource"]
        Resource = [
          for suffix in ["api-5xx", "api-latency", "lambda-duration", "lambda-errors", "lambda-throttles"] :
          "arn:aws:cloudwatch:${local.region}:${local.account_id}:alarm:${configuration.function_name}-${suffix}"
        ]
      },
    ]
  }

  development_mutation_statements = [
    {
      Sid      = "DevelopmentStateWrite"
      Effect   = "Allow"
      Action   = ["s3:PutObject", "s3:DeleteObject"]
      Resource = "${local.state_bucket_arn}/${local.environment_configuration.dev.state_key}"
    },
    {
      Sid    = "DevelopmentReleaseWrite"
      Effect = "Allow"
      Action = [
        "lambda:PublishVersion",
        "lambda:UpdateAlias",
        "lambda:UpdateFunctionCode",
      ]
      Resource = [
        "arn:aws:lambda:${local.region}:${local.account_id}:function:${local.environment_configuration.dev.function_name}",
        "arn:aws:lambda:${local.region}:${local.account_id}:function:${local.environment_configuration.dev.function_name}:live",
      ]
      Condition = {
        StringEquals = {
          "aws:ResourceTag/Environment" = "dev"
          "aws:ResourceTag/ManagedBy"   = local.required_tags.ManagedBy
          "aws:ResourceTag/Platform"    = local.required_tags.Platform
          "aws:ResourceTag/Project"     = local.required_tags.Project
        }
      }
    },
  ]

  environment_policies = {
    for key, configuration in local.environment_configuration : key => jsonencode({
      Version = "2012-10-17"
      Statement = concat(
        local.environment_read_statements[key],
        [for statement in local.development_mutation_statements : statement if configuration.mutable],
      )
    })
  }
}

resource "aws_iam_role" "ci" {
  for_each = local.roles

  name                 = each.value.name
  assume_role_policy   = each.value.trust
  max_session_duration = 3600

  tags = {
    ManagedBy = "opentofu"
    Project   = "portfolio"
    Purpose   = "github-release"
  }
}

resource "aws_iam_role_policy" "release" {
  # Keep the target plan-known; depends_on below preserves creation ordering.
  name = "portfolio-release-builder"
  role = local.roles.release.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["ecr:GetAuthorizationToken"]
        Resource = "*"
        Condition = {
          StringEquals = {
            "aws:RequestedRegion" = local.region
          }
        }
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability",
          "ecr:CompleteLayerUpload",
          "ecr:DescribeImageScanFindings",
          "ecr:DescribeImages",
          "ecr:DescribeRepositories",
          "ecr:GetDownloadUrlForLayer",
          "ecr:InitiateLayerUpload",
          "ecr:ListImages",
          "ecr:PutImage",
          "ecr:UploadLayerPart",
        ]
        Resource = local.ecr_repository_arn
      },
    ]
  })

  depends_on = [aws_iam_role.ci]
}

resource "aws_iam_role_policy" "environment" {
  for_each = local.environment_configuration

  # Keep the target plan-known; depends_on below preserves creation ordering.
  name   = each.value.policy_name
  role   = each.value.role_name
  policy = local.environment_policies[each.key]

  depends_on = [aws_iam_role.ci]
}
