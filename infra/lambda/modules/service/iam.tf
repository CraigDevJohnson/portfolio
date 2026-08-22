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
  name               = "${local.function_name}-execution"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json

  lifecycle {
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
  }
}

resource "aws_iam_role_policy" "lambda" {
  name   = "${local.function_name}-runtime"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.lambda.json
}
