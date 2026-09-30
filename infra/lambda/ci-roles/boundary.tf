# Permissions boundary for the portfolio Lambda execution roles. The service
# roots attach it by ARN, so it must exist before dev or prod is applied. It is
# a ceiling: each statement allows only its own environment's execution role.
# SITE_SESSION_KEY is in each ceiling ahead of site sign-in; the role's own
# policy reads it only once that environment's `site` input is set.
locals {
  boundary_environments = {
    dev = {
      sid        = "Dev"
      parameters = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY", "SITE_SESSION_KEY"]
    }
    prod = {
      sid        = "Prod"
      parameters = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY", "SITE_SESSION_KEY"]
    }
  }

  boundary_statements = flatten([
    for environment, settings in local.boundary_environments : [
      {
        Sid      = "${settings.sid}GoogleConnections"
        Effect   = "Allow"
        Action   = ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"]
        Resource = "arn:aws:dynamodb:${local.region}:${local.account_id}:table/portfolio-lambda-${environment}-google-connections"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
        }
      },
      {
        Sid      = "${settings.sid}SoccerSessions"
        Effect   = "Allow"
        Action   = "dynamodb:PutItem"
        Resource = "arn:aws:dynamodb:${local.region}:${local.account_id}:table/portfolio-lambda-${environment}-soccer-sessions"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
        }
      },
      # Stage 1 of LPS history collection: the HTTP runtime enrolls, reads and
      # removes history. The admission transaction's conditional puts are
      # authorized as dynamodb:PutItem.
      {
        Sid      = "${settings.sid}SoccerHistory"
        Effect   = "Allow"
        Action   = ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:Query"]
        Resource = "arn:aws:dynamodb:${local.region}:${local.account_id}:table/portfolio-lambda-${environment}-soccer-history"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
        }
      },
      {
        Sid    = "${settings.sid}Parameters"
        Effect = "Allow"
        Action = "ssm:GetParameters"
        Resource = [
          for name in settings.parameters :
          "arn:aws:ssm:${local.region}:${local.account_id}:parameter/portfolio/lambda/${environment}/${name}"
        ]
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
        }
      },
      {
        Sid      = "${settings.sid}ParameterDecryption"
        Effect   = "Allow"
        Action   = "kms:Decrypt"
        Resource = "arn:aws:kms:${local.region}:${local.account_id}:key/*"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
          StringEquals = {
            "kms:CallerAccount" = local.account_id
            "kms:ViaService"    = "ssm.${local.region}.amazonaws.com"
            "kms:EncryptionContext:PARAMETER_ARN" = [
              for name in settings.parameters :
              "arn:aws:ssm:${local.region}:${local.account_id}:parameter/portfolio/lambda/${environment}/${name}"
            ]
          }
          "ForAnyValue:StringEquals" = { "kms:ResourceAliases" = "alias/aws/ssm" }
        }
      },
      {
        Sid      = "${settings.sid}LambdaLogs"
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/lambda/portfolio-lambda-${environment}:*"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-execution" }
        }
      },
    ]
  ])

  # Read-only grants for the optional, currently disabled development portal.
  # D22 drops its EC2 start/stop and /ec2/i-* log grants.
  boundary_portal_statements = [
    {
      Sid      = "DevManagementRead"
      Effect   = "Allow"
      Action   = ["ec2:DescribeInstances", "cloudwatch:GetMetricStatistics"]
      Resource = "*"
      Condition = {
        ArnEquals    = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-dev-execution" }
        StringEquals = { "aws:RequestedRegion" = local.region }
      }
    },
  ]
}

resource "aws_iam_policy" "lambda_execution_boundary" {
  name        = "PortfolioLambdaExecutionBoundary"
  path        = "/portfolio/boundaries/"
  description = "Permissions boundary for the portfolio Lambda execution roles"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = concat(local.boundary_statements, local.boundary_portal_statements)
  })
}

# Permissions boundary for the LPS history worker and its Scheduler role
# (stage 2, the daily schedule). It is separate because adding these grants to
# PortfolioLambdaExecutionBoundary for both environments would exceed IAM's
# 6,144-character managed-policy limit. Each statement allows only its own
# environment's worker or Scheduler role, and mirrors that role's own policy in
# modules/service/history_worker.tf.
locals {
  history_boundary_statements = flatten([
    for environment, settings in local.boundary_environments : [
      {
        Sid      = "${settings.sid}HistoryWorkerTable"
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem"]
        Resource = "arn:aws:dynamodb:${local.region}:${local.account_id}:table/portfolio-lambda-${environment}-soccer-history"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-execution" }
        }
      },
      {
        Sid      = "${settings.sid}HistoryWorkerDueIndex"
        Effect   = "Allow"
        Action   = "dynamodb:Query"
        Resource = "arn:aws:dynamodb:${local.region}:${local.account_id}:table/portfolio-lambda-${environment}-soccer-history/index/due-teams"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-execution" }
        }
      },
      {
        Sid      = "${settings.sid}HistoryWorkerLogs"
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "arn:aws:logs:${local.region}:${local.account_id}:log-group:/aws/lambda/portfolio-lambda-${environment}-soccer-history:*"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-execution" }
        }
      },
      {
        Sid      = "${settings.sid}HistoryFailures"
        Effect   = "Allow"
        Action   = "sqs:SendMessage"
        Resource = "arn:aws:sqs:${local.region}:${local.account_id}:portfolio-lambda-${environment}-soccer-history-failures"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = [
            "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-execution",
            "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-scheduler",
          ] }
        }
      },
      {
        Sid      = "${settings.sid}HistoryInvoke"
        Effect   = "Allow"
        Action   = "lambda:InvokeFunction"
        Resource = "arn:aws:lambda:${local.region}:${local.account_id}:function:portfolio-lambda-${environment}-soccer-history"
        Condition = {
          ArnEquals = { "aws:PrincipalArn" = "arn:aws:iam::${local.account_id}:role/portfolio-lambda-${environment}-soccer-history-scheduler" }
        }
      },
    ]
  ])
}

resource "aws_iam_policy" "lambda_history_execution_boundary" {
  name        = "PortfolioLambdaHistoryExecutionBoundary"
  path        = "/portfolio/boundaries/"
  description = "Permissions boundary for the portfolio LPS history worker and Scheduler roles"
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = local.history_boundary_statements
  })
}
