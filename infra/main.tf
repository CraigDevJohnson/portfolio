# ──────────────────────────────────────────────
# Locals
# ──────────────────────────────────────────────

locals {
  google_connection_table_name = "${var.app_name}-google-connections"
  environment                  = "development"
  ssm_parameter_base_arn       = "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/${var.app_name}"
  # Runtime secrets for Google OAuth and soccer JWT session authentication.
  ssm_parameter_names          = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"]
}

# ──────────────────────────────────────────────
# Data sources
# ──────────────────────────────────────────────

data "aws_caller_identity" "current" {}

data "aws_kms_alias" "ssm" {
  name = "alias/aws/ssm"
}

# ──────────────────────────────────────────────
# ECR Repository — stores container images
# ──────────────────────────────────────────────

resource "aws_ecr_repository" "app" {
  name                 = var.app_name
  image_tag_mutability = "MUTABLE"
  # Do not force-delete the repository when it still contains images to avoid accidental loss in production.
  force_delete = false

  image_scanning_configuration {
    scan_on_push = true
  }
}

# Keep only the last 5 untagged images to save storage costs
resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep only last 5 untagged images"
        selection = {
          tagStatus   = "untagged"
          countType   = "imageCountMoreThan"
          countNumber = 5
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

# ──────────────────────────────────────────────
# DynamoDB — persistent Google connection store
# ──────────────────────────────────────────────

resource "aws_dynamodb_table" "google_connections" {
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "connection_id"
  name         = local.google_connection_table_name

  attribute {
    name = "connection_id"
    type = "S"
  }

  tags = {
    Name        = local.google_connection_table_name
    Environment = local.environment
  }
}

# ──────────────────────────────────────────────
# IAM — App Runner access role for ECR
# ──────────────────────────────────────────────

resource "aws_iam_role" "apprunner_ecr_access" {
  name = "${var.app_name}-apprunner-ecr-access"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "build.apprunner.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "apprunner_ecr_access" {
  role       = aws_iam_role.apprunner_ecr_access.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"
}

# ──────────────────────────────────────────────
# IAM — App Runner instance role (for DynamoDB,
# and future SES, Lambda, etc.)
# ──────────────────────────────────────────────

resource "aws_iam_role" "apprunner_instance" {
  name = "${var.app_name}-apprunner-instance"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "tasks.apprunner.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

resource "aws_iam_policy" "google_connections_dynamodb" {
  name = "${var.app_name}-google-connections-dynamodb"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:DeleteItem",
          "dynamodb:DescribeTable",
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:UpdateItem",
        ]
        Resource = aws_dynamodb_table.google_connections.arn
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "google_connections_dynamodb" {
  policy_arn = aws_iam_policy.google_connections_dynamodb.arn
  role       = aws_iam_role.apprunner_instance.name
}

resource "aws_iam_policy" "apprunner_runtime_secrets" {
  name = "${var.app_name}-apprunner-runtime-secrets"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ssm:GetParameter",
          "ssm:GetParameters",
        ]
        Resource = [for name in local.ssm_parameter_names : "${local.ssm_parameter_base_arn}/${name}"]
      },
      {
        Effect   = "Allow"
        Action   = ["kms:Decrypt"]
        Resource = data.aws_kms_alias.ssm.target_key_arn
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "apprunner_runtime_secrets" {
  policy_arn = aws_iam_policy.apprunner_runtime_secrets.arn
  role       = aws_iam_role.apprunner_instance.name
}

# ──────────────────────────────────────────────
# App Runner Service
# ──────────────────────────────────────────────

resource "aws_apprunner_service" "app" {
  service_name = var.app_name

  source_configuration {
    authentication_configuration {
      access_role_arn = aws_iam_role.apprunner_ecr_access.arn
    }

    image_repository {
      image_identifier      = "${aws_ecr_repository.app.repository_url}:${var.ecr_image_tag}"
      image_repository_type = "ECR"

      image_configuration {
        port = tostring(var.container_port)

        runtime_environment_variables = {
          APP_BIND_ALL                 = "true"
          GOOGLE_CONNECTION_TABLE_NAME = local.google_connection_table_name
        }
        runtime_environment_secrets = { for name in local.ssm_parameter_names : name => "${local.ssm_parameter_base_arn}/${name}" }
      }
    }

    auto_deployments_enabled = false
  }

  instance_configuration {
    cpu               = var.app_runner_cpu
    memory            = var.app_runner_memory
    instance_role_arn = aws_iam_role.apprunner_instance.arn
  }

  health_check_configuration {
    protocol            = "HTTP"
    path                = "/"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 10
    timeout             = 5
  }

  tags = {
    Name        = var.app_name
    Environment = local.environment
  }
}
