data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

data "aws_kms_alias" "ssm" {
  name = "alias/aws/ssm"
}

data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name                 = "${local.function_name}-execution"
  assume_role_policy   = data.aws_iam_policy_document.lambda_assume_role.json
  permissions_boundary = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary"

  lifecycle {
    precondition {
      condition     = var.management == null ? true : (var.environment == "dev" && var.name_prefix == "portfolio-lambda-dev" && var.aws_region == "us-west-2" && data.aws_caller_identity.current.account_id == "180294223248" && data.aws_partition.current.partition == "aws")
      error_message = "management is restricted to the reviewed development AWS account and region."
    }

    precondition {
      condition     = !var.activate_custom_domain || var.request_custom_domain
      error_message = "activate_custom_domain requires request_custom_domain"
    }

    precondition {
      condition     = !var.activate_custom_domain || length(var.domain_names) > 0
      error_message = "activate_custom_domain requires at least one domain"
    }
  }
}

data "aws_iam_policy_document" "lambda" {
  statement {
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]
    resources = [aws_dynamodb_table.google_connections.arn]
  }

  statement {
    actions   = ["dynamodb:PutItem"]
    resources = [aws_dynamodb_table.soccer_sessions.arn]
  }

  statement {
    actions   = ["ssm:GetParameters"]
    resources = [for path in values(local.ssm_paths) : "arn:${data.aws_partition.current.partition}:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${path}"]
  }

  statement {
    actions   = ["kms:Decrypt"]
    resources = [data.aws_kms_alias.ssm.target_key_arn]
    dynamic "condition" {
      for_each = var.management == null ? [] : [1]
      content {
        test     = "StringEquals"
        variable = "kms:EncryptionContext:PARAMETER_ARN"
        values   = [for path in values(local.ssm_paths) : "arn:aws:ssm:us-west-2:180294223248:parameter${path}"]
      }
    }
  }

  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.lambda.arn}:*"]
  }
  dynamic "statement" {
    for_each = var.management == null ? [] : [1]
    content {
      actions   = ["ec2:DescribeInstances", "cloudwatch:GetMetricStatistics"]
      resources = ["*"]
      condition {
        test     = "StringEquals"
        variable = "aws:RequestedRegion"
        values   = ["us-west-2"]
      }
    }
  }
  dynamic "statement" {
    for_each = var.management == null ? [] : [1]
    content {
      actions   = ["logs:FilterLogEvents"]
      resources = ["arn:aws:logs:us-west-2:180294223248:log-group:/ec2/i-*:*"]
    }
  }
  dynamic "statement" {
    for_each = var.management == null ? [] : [1]
    content {
      actions   = ["ec2:StartInstances", "ec2:StopInstances"]
      resources = ["arn:aws:ec2:us-west-2:180294223248:instance/*"]
      condition {
        test     = "StringEquals"
        variable = "ec2:ResourceTag/PortfolioManagement"
        values   = ["dev"]
      }
    }
  }

}

resource "aws_iam_role_policy" "lambda" {
  name   = "${local.function_name}-runtime"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.lambda.json
}
