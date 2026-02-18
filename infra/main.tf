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
# IAM — App Runner instance role (for future
# DynamoDB, SES, Lambda, etc.)
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

# Future: attach policies here for DynamoDB, SES, etc.
# resource "aws_iam_role_policy_attachment" "dynamodb_access" {
#   role       = aws_iam_role.apprunner_instance.name
#   policy_arn = aws_iam_policy.dynamodb_access.arn
# }

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
    healthy_threshold   = 2  # consecutive successes to mark healthy
    unhealthy_threshold = 3  # consecutive failures to mark unhealthy
    interval            = 10 # seconds between checks
    timeout             = 5  # seconds to wait before considering the check failed
  }

  tags = {
    Name        = var.app_name
    Environment = "development"
  }
}
