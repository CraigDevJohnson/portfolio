mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = { account_id = "111122223333" }
  }

  mock_data "aws_iam_openid_connect_provider" {
    defaults = { arn = "arn:aws:iam::111122223333:oidc-provider/token.actions.githubusercontent.com" }
  }

  mock_data "aws_iam_policy_document" {
    defaults = {
      json = <<-JSON
        {
          "Version": "2012-10-17",
          "Statement": [{
            "Effect": "Allow",
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Principal": {
              "Federated": "arn:aws:iam::111122223333:oidc-provider/token.actions.githubusercontent.com"
            }
          }]
        }
      JSON
    }
  }
}

variables {
  aws_account_id = "111122223333"
  # Mock providers cannot import; the bucket's configured arguments are still
  # planned and asserted below.
  import_state_bucket = false
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
      "lambda:GetFunctionEventInvokeConfig",
      "lambda:GetPolicy",
      "lambda:GetRuntimeManagementConfig",
      "lambda:ListTags",
      "lambda:ListVersionsByFunction",
      "lambda:PublishVersion",
      "lambda:UpdateAlias",
      "lambda:UpdateFunctionCode",
      "logs:DescribeLogGroups",
      "logs:DescribeMetricFilters",
      "logs:ListTagsForResource",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetObject",
      "s3:ListBucket",
      "s3:PutObject",
      "scheduler:GetSchedule",
      "sqs:GetQueueAttributes",
      "sqs:ListQueueTags",
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
        "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev",
        "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev:live",
      ]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement :
        statement.Condition.StringEquals
        if statement.Sid == "DevelopmentReleaseWrite"
        ]) == {
        "aws:ResourceTag/Environment" = "dev"
        "aws:ResourceTag/ManagedBy"   = "opentofu"
        "aws:ResourceTag/Platform"    = "lambda-http-api"
        "aws:ResourceTag/project"     = "portfolio"
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
      ]).Resource == "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/dev/terraform.tfstate" &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateLock"
      ]).Action) == toset(["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "StateLock"
        ]).Resource == join("", [
        "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/dev/",
        "terraform.tfstate.tflock",
      ]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "DevelopmentStateWrite"
      ]).Action) == toset(["s3:PutObject", "s3:DeleteObject"]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "DevelopmentStateWrite"
      ]).Resource == "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/dev/terraform.tfstate"
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
    condition = alltrue([
      for environment, function_name in {
        dev  = "portfolio-lambda-dev"
        prod = "portfolio-lambda-prod"
        } : (
        toset(one([
          for statement in jsondecode(aws_iam_role_policy.environment[environment].policy).Statement :
          try(tolist(statement.Action), [statement.Action])
          if statement.Sid == "AlarmRead"
        ])) == toset(["cloudwatch:DescribeAlarms", "cloudwatch:ListTagsForResource"]) &&
        toset(one([
          for statement in jsondecode(aws_iam_role_policy.environment[environment].policy).Statement :
          try(tolist(statement.Resource), [statement.Resource])
          if statement.Sid == "AlarmRead"
          ])) == toset([
          for suffix in [
            "api-5xx",
            "api-latency",
            "lambda-duration",
            "lambda-errors",
            "lambda-throttles",
            "soccer-history-admission-rejected",
            "soccer-history-incomplete",
            "soccer-history-errors",
            "soccer-history-dead-letter",
          ] : "arn:aws:cloudwatch:us-west-2:111122223333:alarm:${function_name}-${suffix}"
        ])
      )
    ])
    error_message = "alarm refresh must remain scoped to the five exact environment alarms and the four history alarms"
  }

  # LPS history (readiness packet 6.1 item 4): every CI role reads its own
  # environment's history resources, so a release plan can still refresh
  # state once a history stage is applied. Stage 2 resources do not exist yet.
  assert {
    condition = alltrue(flatten([
      for role in [
        { statements = jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement, f = "portfolio-lambda-dev" },
        { statements = jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement, f = "portfolio-lambda-prod" },
        { statements = jsondecode(aws_iam_role_policy.production_deployer.policy).Statement, f = "portfolio-lambda-prod" },
        ] : [
        for expected in [
          {
            sid     = "TableRead"
            actions = ["dynamodb:DescribeContinuousBackups", "dynamodb:DescribeTable", "dynamodb:DescribeTimeToLive", "dynamodb:ListTagsOfResource"]
            resources = [
              "arn:aws:dynamodb:us-west-2:111122223333:table/${role.f}-google-connections",
              "arn:aws:dynamodb:us-west-2:111122223333:table/${role.f}-soccer-sessions",
              "arn:aws:dynamodb:us-west-2:111122223333:table/${role.f}-soccer-history",
            ]
          },
          {
            sid     = "MetricFilterRead"
            actions = ["logs:DescribeMetricFilters"]
            resources = [
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}:*",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}-soccer-history",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}-soccer-history:*",
            ]
          },
          {
            sid     = "ExecutionRoleRead"
            actions = ["iam:GetRole", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies", "iam:ListRolePolicies", "iam:ListRoleTags"]
            resources = [
              "arn:aws:iam::111122223333:role/${role.f}-execution",
              "arn:aws:iam::111122223333:role/${role.f}-soccer-history-execution",
              "arn:aws:iam::111122223333:role/${role.f}-soccer-history-scheduler",
            ]
          },
          {
            sid = "LambdaRead"
            actions = [
              "lambda:GetAlias", "lambda:GetFunction", "lambda:GetFunctionCodeSigningConfig", "lambda:GetFunctionConcurrency",
              "lambda:GetFunctionConfiguration", "lambda:GetPolicy", "lambda:GetRuntimeManagementConfig", "lambda:ListTags",
              "lambda:ListVersionsByFunction",
            ]
            resources = [
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}",
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}:*",
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}-soccer-history",
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}-soccer-history:*",
            ]
          },
          {
            sid     = "LogGroupRead"
            actions = ["logs:ListTagsForResource"]
            resources = [
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/apigateway/${role.f}/access",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/apigateway/${role.f}/access:*",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}:*",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}-soccer-history",
              "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/${role.f}-soccer-history:*",
            ]
          },
          {
            sid     = "HistoryWorkerInvokeConfigRead"
            actions = ["lambda:GetFunctionEventInvokeConfig"]
            resources = [
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}-soccer-history",
              "arn:aws:lambda:us-west-2:111122223333:function:${role.f}-soccer-history:*",
            ]
          },
          {
            sid       = "HistoryQueueRead"
            actions   = ["sqs:GetQueueAttributes", "sqs:ListQueueTags"]
            resources = ["arn:aws:sqs:us-west-2:111122223333:${role.f}-soccer-history-failures"]
          },
          {
            sid       = "HistoryScheduleRead"
            actions   = ["scheduler:GetSchedule"]
            resources = ["arn:aws:scheduler:us-west-2:111122223333:schedule/default/${role.f}-soccer-history-daily"]
          },
          ] : length([
            for statement in role.statements : statement
            if statement.Sid == expected.sid && statement.Effect == "Allow" && !contains(keys(statement), "Condition") &&
            toset(try(tolist(statement.Action), [statement.Action])) == toset(expected.actions) &&
            toset(try(tolist(statement.Resource), [statement.Resource])) == toset(expected.resources)
        ]) == 1
      ]
    ]))
    error_message = "every CI role must read exactly its own environment's history table, metric filters, roles, worker, log groups, invoke configuration, failure queue and schedule"
  }

  # A release moves the history worker to the release image, so each deployer
  # may publish only that worker's code, under the same tag conditions as the
  # service function. The production planner writes nothing.
  assert {
    condition = (
      [
        for statement in jsondecode(aws_iam_role_policy.environment["dev"].policy).Statement : statement
        if statement.Sid == "DevelopmentHistoryWorkerReleaseWrite"
        ] == [{
          Sid      = "DevelopmentHistoryWorkerReleaseWrite"
          Effect   = "Allow"
          Action   = ["lambda:PublishVersion", "lambda:UpdateFunctionCode"]
          Resource = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-dev-soccer-history"
          Condition = { StringEquals = {
            "aws:ResourceTag/Environment" = "dev"
            "aws:ResourceTag/ManagedBy"   = "opentofu"
            "aws:ResourceTag/Platform"    = "lambda-http-api"
            "aws:ResourceTag/project"     = "portfolio"
          } }
      }] &&
      [
        for statement in jsondecode(aws_iam_role_policy.production_deployer.policy).Statement : statement
        if statement.Sid == "ProductionHistoryWorkerReleaseWrite"
        ] == [{
          Sid      = "ProductionHistoryWorkerReleaseWrite"
          Effect   = "Allow"
          Action   = ["lambda:PublishVersion", "lambda:UpdateFunctionCode"]
          Resource = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod-soccer-history"
          Condition = { StringEquals = {
            "aws:ResourceTag/Environment" = "prod"
            "aws:ResourceTag/ManagedBy"   = "opentofu"
            "aws:ResourceTag/Platform"    = "lambda-http-api"
            "aws:ResourceTag/project"     = "portfolio"
          } }
      }] &&
      length([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement : statement
        if endswith(statement.Sid, "Write")
      ]) == 0
    )
    error_message = "each deployer may release only its own environment's history worker image, and the production planner writes nothing"
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
      "lambda:GetFunctionEventInvokeConfig",
      "lambda:GetPolicy",
      "lambda:GetRuntimeManagementConfig",
      "lambda:ListTags",
      "lambda:ListVersionsByFunction",
      "logs:DescribeLogGroups",
      "logs:DescribeMetricFilters",
      "logs:ListTagsForResource",
      "s3:DeleteObject",
      "s3:GetBucketLocation",
      "s3:GetBucketVersioning",
      "s3:GetObject",
      "s3:ListBucket",
      "s3:PutObject",
      "scheduler:GetSchedule",
      "sqs:GetQueueAttributes",
      "sqs:ListQueueTags",
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
        "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/prod/",
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
        "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/prod/",
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
      length(aws_iam_role_policy.environment["prod"].policy) <= 10240 &&
      length(aws_iam_role_policy.production_deployer.policy) <= 10240
    )
    error_message = "environment inline policies must fit the IAM role-policy size limit"
  }

  assert {
    condition = (
      aws_iam_role.production_deployer.name == "portfolio-production-deployer-ci" &&
      aws_iam_role_policy.production_deployer.name == "portfolio-production-runtime-release"
    )
    error_message = "production deployment must use a separate deterministic role and policy"
  }

  assert {
    condition = toset(flatten([
      for statement in jsondecode(aws_iam_role_policy.production_deployer.policy).Statement :
      try(tolist(statement.Action), [statement.Action])
      ])) == setunion(
      toset(flatten([
        for statement in jsondecode(aws_iam_role_policy.environment["prod"].policy).Statement :
        try(tolist(statement.Action), [statement.Action])
      ])),
      toset([
        "lambda:PublishVersion",
        "lambda:UpdateAlias",
        "lambda:UpdateFunctionCode",
      ]),
    )
    error_message = "production deployment adds only exact state and Lambda release writes"
  }

  assert {
    condition = (
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.production_deployer.policy).Statement :
        try(tolist(statement.Action), [statement.Action])
        if statement.Sid == "ProductionReleaseWrite"
      ])) == toset(["lambda:PublishVersion", "lambda:UpdateAlias", "lambda:UpdateFunctionCode"]) &&
      toset(one([
        for statement in jsondecode(aws_iam_role_policy.production_deployer.policy).Statement :
        try(tolist(statement.Resource), [statement.Resource])
        if statement.Sid == "ProductionReleaseWrite"
        ])) == toset([
        "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod",
        "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-prod:live",
      ]) &&
      one([
        for statement in jsondecode(aws_iam_role_policy.production_deployer.policy).Statement :
        statement.Resource
        if statement.Sid == "ProductionStateWrite"
      ]) == "arn:aws:s3:::portfolio-tofu-state-111122223333/portfolio-lambda-http-api/prod/terraform.tfstate"
    )
    error_message = "production writes must be limited to the exact state object and existing Lambda release resources"
  }

  assert {
    condition = alltrue([
      for condition in data.aws_iam_policy_document.production_deployer_trust.statement[0].condition :
      condition.test == "StringEquals" && (
        (
          condition.variable == "token.actions.githubusercontent.com:aud" &&
          toset(condition.values) == toset(["sts.amazonaws.com"])
          ) || (
          condition.variable == "token.actions.githubusercontent.com:sub" &&
          toset(condition.values) == toset([
            "repo:CraigDevJohnson/portfolio:environment:production",
          ])
        )
      )
    ]) && length(data.aws_iam_policy_document.production_deployer_trust.statement[0].condition) == 2
    error_message = "production deployer trust must bind the exact audience, repository, and production environment"
  }
}

run "oidc_trust_uses_the_account_provider" {
  command = plan

  assert {
    condition = alltrue([
      for document in concat(
        [data.aws_iam_policy_document.release_trust, data.aws_iam_policy_document.production_deployer_trust],
        values(data.aws_iam_policy_document.environment_trust),
        ) : alltrue([
          for principal in document.statement[0].principals :
          principal.type == "Federated" &&
          toset(principal.identifiers) == toset(["arn:aws:iam::111122223333:oidc-provider/token.actions.githubusercontent.com"])
      ])
    ])
    error_message = "every CI role must trust the OIDC provider looked up by URL"
  }

  assert {
    condition = (
      toset(one([
        for condition in data.aws_iam_policy_document.release_trust.statement[0].condition : condition.values
        if condition.variable == "token.actions.githubusercontent.com:sub"
      ])) == toset(["repo:CraigDevJohnson/portfolio:ref:refs/heads/main"]) &&
      toset(one([
        for condition in data.aws_iam_policy_document.environment_trust["dev"].statement[0].condition : condition.values
        if condition.variable == "token.actions.githubusercontent.com:sub"
      ])) == toset(["repo:CraigDevJohnson/portfolio:environment:development"]) &&
      toset(one([
        for condition in data.aws_iam_policy_document.environment_trust["prod"].statement[0].condition : condition.values
        if condition.variable == "token.actions.githubusercontent.com:sub"
      ])) == toset(["repo:CraigDevJohnson/portfolio:environment:production-plan"])
    )
    error_message = "the CI roles keep their default-format sub claims"
  }

  assert {
    condition = alltrue([
      for role in concat(values(aws_iam_role.ci), [aws_iam_role.production_deployer]) :
      role.tags == tomap({ Purpose = "github-release" }) && role.max_session_duration == 3600
    ])
    error_message = "CI roles keep one-hour sessions and take the project tag from the provider"
  }
}

run "execution_boundary_contract" {
  command = plan

  assert {
    condition = (
      aws_iam_policy.lambda_execution_boundary.name == "PortfolioLambdaExecutionBoundary" &&
      aws_iam_policy.lambda_execution_boundary.path == "/portfolio/boundaries/"
    )
    error_message = "the boundary keeps the name and path the service roots attach"
  }

  # IAM enforces the managed-policy size only when an apply calls
  # CreatePolicyVersion; a plan never submits the document, so check it here.
  # The mocked account ID has the same length as the real one.
  assert {
    condition     = length(aws_iam_policy.lambda_execution_boundary.policy) <= 6144
    error_message = "the boundary must fit IAM's 6,144-character managed-policy limit"
  }

  assert {
    condition = toset([
      for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement : statement.Sid
      ]) == toset([
      "DevGoogleConnections", "DevSoccerSessions", "DevSoccerHistory", "DevParameters", "DevParameterDecryption", "DevLambdaLogs",
      "ProdGoogleConnections", "ProdSoccerSessions", "ProdSoccerHistory", "ProdParameters", "ProdParameterDecryption", "ProdLambdaLogs",
      "DevManagementRead",
    ])
    error_message = "the boundary holds exactly the reviewed runtime statements"
  }

  # Stage 1 of LPS history collection (readiness packet 6.1 item 3): the HTTP
  # runtime enrolls, reads and removes history. The admission transaction's
  # conditional puts are authorized as dynamodb:PutItem.
  assert {
    condition = alltrue([
      for environment, sid in { dev = "Dev", prod = "Prod" } : (
        one([
          for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement : statement
          if statement.Sid == "${sid}SoccerHistory"
          ]) == {
          Sid       = "${sid}SoccerHistory"
          Effect    = "Allow"
          Action    = ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query"]
          Resource  = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-lambda-${environment}-soccer-history"
          Condition = { ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-execution" } }
        }
      )
    ])
    error_message = "each HTTP execution role may read, enroll into and remove from only its own environment's history table"
  }

  assert {
    condition = alltrue([
      for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement :
      statement.Effect == "Allow" &&
      statement.Condition.ArnEquals["aws:PrincipalArn"] == (
        startswith(statement.Sid, "Prod") ?
        "arn:aws:iam::111122223333:role/portfolio-lambda-prod-execution" :
        "arn:aws:iam::111122223333:role/portfolio-lambda-dev-execution"
      )
    ])
    error_message = "each boundary statement applies only to its own environment's execution role"
  }

  assert {
    condition = length([
      for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement : statement
      if length(setintersection(
        toset(try(tolist(statement.Action), [statement.Action])),
        toset(["ec2:StartInstances", "ec2:StopInstances", "logs:FilterLogEvents"]),
      )) > 0
    ]) == 0
    error_message = "D22: the boundary grants no EC2 start/stop or instance log reads"
  }

  assert {
    condition = alltrue([
      for environment, sid in { dev = "Dev", prod = "Prod" } : (
        one([
          for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement : statement.Resource
          if statement.Sid == "${sid}Parameters"
          ]) == [
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/CLIENT_ID_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/CLIENT_SECRET_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/LPS_SESSION_KEY",
          "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/SITE_SESSION_KEY",
        ] &&
        one([
          for statement in jsondecode(aws_iam_policy.lambda_execution_boundary.policy).Statement : statement.Condition.StringEquals
          if statement.Sid == "${sid}ParameterDecryption"
          ]) == {
          "kms:CallerAccount" = "111122223333"
          "kms:ViaService"    = "ssm.us-west-2.amazonaws.com"
          "kms:EncryptionContext:PARAMETER_ARN" = [
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/CLIENT_ID_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/CLIENT_SECRET_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/LPS_SESSION_KEY",
            "arn:aws:ssm:us-west-2:111122223333:parameter/portfolio/lambda/${environment}/SITE_SESSION_KEY",
          ]
        }
      )
    ])
    error_message = "each environment's parameter reads and decryption are limited to its four SecureStrings, including its own SITE_SESSION_KEY; the retired MGMT_SESSION_KEY is not readable"
  }
}

# Stage 2 of LPS history collection (readiness packet 6.1 item 3): the daily
# worker and its Scheduler role attach a second boundary, because both stages in
# one policy would exceed IAM's managed-policy limit.
run "history_execution_boundary_contract" {
  command = plan

  assert {
    condition = (
      aws_iam_policy.lambda_history_execution_boundary.name == "PortfolioLambdaHistoryExecutionBoundary" &&
      aws_iam_policy.lambda_history_execution_boundary.path == "/portfolio/boundaries/"
    )
    error_message = "the history boundary keeps the name and path the service module attaches to the worker and Scheduler roles"
  }

  assert {
    condition     = length(aws_iam_policy.lambda_history_execution_boundary.policy) <= 6144
    error_message = "the history boundary must fit IAM's 6,144-character managed-policy limit"
  }

  assert {
    condition = jsondecode(aws_iam_policy.lambda_history_execution_boundary.policy).Statement == flatten([
      for environment, sid in { dev = "Dev", prod = "Prod" } : [
        {
          Sid       = "${sid}HistoryWorkerTable"
          Effect    = "Allow"
          Action    = ["dynamodb:GetItem", "dynamodb:PutItem"]
          Resource  = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-lambda-${environment}-soccer-history"
          Condition = { ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-execution" } }
        },
        {
          Sid       = "${sid}HistoryWorkerDueIndex"
          Effect    = "Allow"
          Action    = "dynamodb:Query"
          Resource  = "arn:aws:dynamodb:us-west-2:111122223333:table/portfolio-lambda-${environment}-soccer-history/index/due-teams"
          Condition = { ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-execution" } }
        },
        {
          Sid       = "${sid}HistoryWorkerLogs"
          Effect    = "Allow"
          Action    = ["logs:CreateLogStream", "logs:PutLogEvents"]
          Resource  = "arn:aws:logs:us-west-2:111122223333:log-group:/aws/lambda/portfolio-lambda-${environment}-soccer-history:*"
          Condition = { ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-execution" } }
        },
        {
          Sid      = "${sid}HistoryFailures"
          Effect   = "Allow"
          Action   = "sqs:SendMessage"
          Resource = "arn:aws:sqs:us-west-2:111122223333:portfolio-lambda-${environment}-soccer-history-failures"
          Condition = { ArnEquals = { "aws:PrincipalArn" = [
            "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-execution",
            "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-scheduler",
          ] } }
        },
        {
          Sid       = "${sid}HistoryInvoke"
          Effect    = "Allow"
          Action    = "lambda:InvokeFunction"
          Resource  = "arn:aws:lambda:us-west-2:111122223333:function:portfolio-lambda-${environment}-soccer-history"
          Condition = { ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::111122223333:role/portfolio-lambda-${environment}-soccer-history-scheduler" } }
        },
      ]
    ])
    error_message = "the history boundary lets each environment's worker reach only its table, due index, log group and failure queue, and its Scheduler role only invoke that worker and report to that queue"
  }
}

run "state_bucket_contract" {
  command = plan

  assert {
    condition = (
      aws_s3_bucket.state.bucket == "portfolio-tofu-state-111122223333" &&
      aws_s3_bucket_versioning.state.versioning_configuration[0].status == "Enabled" &&
      aws_s3_bucket_public_access_block.state.block_public_acls &&
      aws_s3_bucket_public_access_block.state.block_public_policy &&
      aws_s3_bucket_public_access_block.state.ignore_public_acls &&
      aws_s3_bucket_public_access_block.state.restrict_public_buckets &&
      one(one(aws_s3_bucket_server_side_encryption_configuration.state.rule).apply_server_side_encryption_by_default).sse_algorithm == "AES256" &&
      tolist(one(aws_s3_bucket_server_side_encryption_configuration.state.rule).blocked_encryption_types) == tolist(["SSE-C"]) &&
      one(aws_s3_bucket_ownership_controls.state.rule).object_ownership == "BucketOwnerEnforced"
    )
    error_message = "the state bucket stays versioned, private, SSE-S3 encrypted and owner-enforced"
  }
}

