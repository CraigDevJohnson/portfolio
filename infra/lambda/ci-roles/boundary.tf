# Permissions boundary for the portfolio Lambda execution roles. The service
# roots attach it by ARN, so it must exist before dev or prod is applied. It is
# a ceiling: each statement allows only its own environment's execution role.
locals {
  boundary_environments = {
    dev = {
      sid = "Dev"
      # The disabled portal's MGMT_SESSION_KEY does not exist. Enabling the
      # portal means creating it and adding it here first.
      parameters = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"]
    }
    prod = {
      sid        = "Prod"
      parameters = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"]
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
