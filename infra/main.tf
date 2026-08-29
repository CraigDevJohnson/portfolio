# ──────────────────────────────────────────────
# Locals
# ──────────────────────────────────────────────

locals {
  google_connection_table_name = "${var.app_name}-google-connections"
  soccer_session_table_name    = "${var.app_name}-soccer-sessions"
  ssm_parameter_base_arn       = "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/${var.app_name}"
  # Runtime secrets for Google OAuth and soccer JWT session authentication.
  ssm_parameter_names = ["CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "LPS_SESSION_KEY"]
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
  # checkov:skip=CKV_AWS_51:Mutable tags required by deployment workflow
  # checkov:skip=CKV_AWS_136:KMS encryption not needed for personal portfolio
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
  # checkov:skip=CKV_AWS_119:KMS encryption not needed for personal portfolio
  # checkov:skip=CKV_AWS_28:Point-in-time recovery not needed for personal portfolio
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "connection_id"
  name         = local.google_connection_table_name

  attribute {
    name = "connection_id"
    type = "S"
  }

  tags = {
    Name        = local.google_connection_table_name
    Environment = var.environment
  }
}

# ──────────────────────────────────────────────
# DynamoDB — soccer session store
# ──────────────────────────────────────────────

resource "aws_dynamodb_table" "soccer_sessions" {
  # checkov:skip=CKV_AWS_119:KMS encryption not needed for personal portfolio
  # checkov:skip=CKV_AWS_28:Point-in-time recovery not needed for personal portfolio
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "session_id"
  name         = local.soccer_session_table_name

  attribute {
    name = "session_id"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  tags = {
    Name        = local.soccer_session_table_name
    Environment = var.environment
  }
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

resource "aws_iam_policy" "soccer_sessions_dynamodb" {
  name = "${var.app_name}-soccer-sessions-dynamodb"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:DeleteItem",
          "dynamodb:GetItem",
          "dynamodb:PutItem",
        ]
        Resource = aws_dynamodb_table.soccer_sessions.arn
      },
    ]
  })
}
