mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = <<-JSON
        {
          "Version": "2012-10-17",
          "Statement": [{
            "Effect": "Allow",
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Principal": {
              "Federated": "arn:aws:iam::180294223248:oidc-provider/token.actions.githubusercontent.com"
            }
          }]
        }
      JSON
    }
  }
}

run "least_privilege_release_roles" {
  command = plan

  assert {
    condition = (
      aws_iam_role_policy.environment["dev"].name == "portfolio-development-runtime-release" &&
      aws_iam_role_policy.environment["prod"].name == "portfolio-production-read-only-plan"
    )
    error_message = "environment inline-policy names must describe their distinct authority"
  }

  assert {
    condition = alltrue([
      for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
      !contains(keys(try(statement.Condition, {})), "StringEqualsIfExists")
    ])
    error_message = "development policy must not use optional conditions"
  }

  assert {
    condition = toset(flatten([
      for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
      try(tolist(statement.Action), [statement.Action])
      ])) == toset([
      "acm:DescribeCertificate",
      "acm:ListTagsForCertificate",
      "apigateway:GET",
      "cloudwatch:DescribeAlarms",
      "cloudwatch:ListTagsForResource",
      "dynamodb:DescribeContinuousBackups",
      "dynamodb:DescribeTable",
      "dynamodb:DescribeTimeToLive",
      "dynamodb:ListTagsOfResource",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:GetDownloadUrlForLayer",
      "iam:GetRole",
      "iam:GetRolePolicy",
      "iam:ListAttachedRolePolicies",
      "iam:ListRolePolicies",
      "iam:ListRoleTags",
      "kms:DescribeKey",
      "kms:ListAliases",
      "lambda:GetAlias",
      "lambda:GetFunction",
      "lambda:GetFunctionCodeSigningConfig",
      "lambda:GetFunctionConcurrency",
      "lambda:GetFunctionConfiguration",
      "lambda:GetPolicy",
      "lambda:GetRuntimeManagementConfig",
      "lambda:ListTags",
      "lambda:ListVersionsByFunction",
      "lambda:PublishVersion",
      "lambda:UpdateAlias",
      "lambda:UpdateFunctionCode",
      "logs:DescribeLogGroups",
      "logs:ListTagsForResource",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetObject",
      "s3:ListBucket",
      "s3:PutObject",
      "sts:GetCallerIdentity",
    ])
    error_message = "development automation must have only refresh, state, and immutable-image release actions"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        try(tolist(statement.Action), [statement.Action])
        if statement.Sid == "DevelopmentReleaseWrite"
      ])) == toset(["lambda:PublishVersion", "lambda:UpdateAlias", "lambda:UpdateFunctionCode"]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        try(tolist(statement.Resource), [statement.Resource])
        if statement.Sid == "DevelopmentReleaseWrite"
        ])) == toset([
        "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev",
        "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-dev:live",
      ]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        statement.Condition.StringEquals
        if statement.Sid == "DevelopmentReleaseWrite"
        ]) == {
        "aws:ResourceTag/Environment" = "dev"
        "aws:ResourceTag/ManagedBy"   = "opentofu"
        "aws:ResourceTag/Platform"    = "lambda-http-api"
        "aws:ResourceTag/Project"     = "portfolio"
      }
    )
    error_message = "development writes must be limited to releasing the existing exact Lambda function"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateRead"
      ]).Action) == toset(["s3:GetObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateRead"
      ]).Resource == "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate" &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateLock"
      ]).Action) == toset(["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateLock"
        ]).Resource == join("", [
        "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/",
        "terraform.tfstate.tflock",
      ]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "DevelopmentStateWrite"
      ]).Action) == toset(["s3:PutObject", "s3:DeleteObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "DevelopmentStateWrite"
      ]).Resource == "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate"
    )
    error_message = "development state access must be limited to the exact state and lock objects"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        try(tolist(statement.Action), [statement.Action])
        if statement.Sid == "KmsAliasList"
      ])) == toset(["kms:ListAliases"]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        try(tolist(statement.Action), [statement.Action])
        if statement.Sid == "KmsSsmKeyRead"
      ])) == toset(["kms:DescribeKey"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        statement.Condition["ForAnyValue:StringEquals"]["kms:ResourceAliases"]
        if statement.Sid == "KmsSsmKeyRead"
      ]) == "alias/aws/ssm"
    )
    error_message = "refresh must have only the bounded KMS alias and key reads"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        try(tolist(statement.Action), [statement.Action])
        if statement.Sid == "CertificateRead"
      ])) == toset(["acm:DescribeCertificate", "acm:ListTagsForCertificate"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        statement.Condition.StringEquals["aws:ResourceTag/Environment"]
        if statement.Sid == "CertificateRead"
      ]) == "dev"
    )
    error_message = "certificate refresh must be read-only and environment constrained"
  }

  assert {
    condition = toset(flatten([
      for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement :
      try(tolist(statement.Action), [statement.Action])
      ])) == toset([
      "acm:DescribeCertificate",
      "acm:ListTagsForCertificate",
      "apigateway:GET",
      "cloudwatch:DescribeAlarms",
      "cloudwatch:ListTagsForResource",
      "dynamodb:DescribeContinuousBackups",
      "dynamodb:DescribeTable",
      "dynamodb:DescribeTimeToLive",
      "dynamodb:ListTagsOfResource",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:GetDownloadUrlForLayer",
      "iam:GetRole",
      "iam:GetRolePolicy",
      "iam:ListAttachedRolePolicies",
      "iam:ListRolePolicies",
      "iam:ListRoleTags",
      "kms:DescribeKey",
      "kms:ListAliases",
      "lambda:GetAlias",
      "lambda:GetFunction",
      "lambda:GetFunctionCodeSigningConfig",
      "lambda:GetFunctionConcurrency",
      "lambda:GetFunctionConfiguration",
      "lambda:GetPolicy",
      "lambda:GetRuntimeManagementConfig",
      "lambda:ListTags",
      "lambda:ListVersionsByFunction",
      "logs:DescribeLogGroups",
      "logs:ListTagsForResource",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetObject",
      "s3:ListBucket",
      "s3:PutObject",
      "sts:GetCallerIdentity",
    ])
    error_message = "production planning must have only the exact refresh and state-lock allowlist"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement : statement
        if statement.Sid == "StateRead"
      ]).Action) == toset(["s3:GetObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement : statement
        if statement.Sid == "StateRead"
        ]).Resource == join("", [
        "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/",
        "terraform.tfstate",
      ]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement : statement
        if statement.Sid == "StateLock"
      ]).Action) == toset(["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement : statement
        if statement.Sid == "StateLock"
        ]).Resource == join("", [
        "arn:aws:s3:::portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/",
        "terraform.tfstate.tflock",
      ])
    )
    error_message = "production planning may read exact state and mutate only its exact lock object"
  }

  assert {
    condition     = local.environment_configuration.prod.github_environment == "production-plan"
    error_message = "production planning trust must bind the exact production-plan environment"
  }

  assert {
    condition = (
      length(aws_iam_role_policy.environment["dev"].policy) <= 10240 &&
      length(aws_iam_role_policy.environment["prod"].policy) <= 10240
    )
    error_message = "environment inline policies must fit the IAM role-policy size limit"
  }
}
