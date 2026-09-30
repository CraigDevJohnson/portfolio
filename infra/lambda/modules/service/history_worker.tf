# All resources in this file are absent until the history table, reviewed
# limits, collection and schedule activation, and an explicit daily
# expression are supplied together. The admission-rejection alert follows
# collection alone.
resource "aws_sqs_queue" "history_dead_letter" {
  count = local.history_schedule_enabled ? 1 : 0

  name                      = "${local.function_name}-soccer-history-failures"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}

resource "aws_cloudwatch_log_group" "history_worker" {
  count             = local.history_schedule_enabled ? 1 : 0
  name              = "/aws/lambda/${local.function_name}-soccer-history"
  retention_in_days = var.log_retention_days
}

data "aws_iam_policy_document" "history_worker_assume" {
  count = local.history_schedule_enabled ? 1 : 0

  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "history_worker" {
  count                = local.history_schedule_enabled ? 1 : 0
  name                 = "${local.function_name}-soccer-history-execution"
  assume_role_policy   = data.aws_iam_policy_document.history_worker_assume[0].json
  permissions_boundary = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary"
}

data "aws_iam_policy_document" "history_worker" {
  count = local.history_schedule_enabled ? 1 : 0

  statement {
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem"]
    resources = [aws_dynamodb_table.soccer_history[0].arn]
  }

  statement {
    actions   = ["dynamodb:Query"]
    resources = ["${aws_dynamodb_table.soccer_history[0].arn}/index/due-teams"]
  }

  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.history_worker[0].arn}:*"]
  }

  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.history_dead_letter[0].arn]
  }
}

resource "aws_iam_role_policy" "history_worker" {
  count  = local.history_schedule_enabled ? 1 : 0
  name   = "${local.function_name}-soccer-history-runtime"
  role   = aws_iam_role.history_worker[0].id
  policy = data.aws_iam_policy_document.history_worker[0].json
}

resource "aws_lambda_function" "history_worker" {
  count         = local.history_schedule_enabled ? 1 : 0
  function_name = "${local.function_name}-soccer-history"
  role          = aws_iam_role.history_worker[0].arn
  package_type  = "Image"
  architectures = ["x86_64"]
  image_uri     = local.image_uri
  memory_size   = var.lambda_memory_mb
  timeout       = var.soccer_history_limits.worker_timeout_seconds
  # One execution at a time, so a repeated or re-driven delivery never runs
  # alongside the first and spends the day's request budget twice. The
  # account's concurrency limit must leave room for it before this stage is
  # applied; see infra/lambda/README.md.
  reserved_concurrent_executions = 1
  publish                        = true

  environment {
    variables = merge(local.history_limit_environment, {
      SOCCER_HISTORY_MODE       = "scheduled"
      SOCCER_ARCHIVE_TABLE_NAME = aws_dynamodb_table.soccer_history[0].name
      LOG_FORMAT                = "json"
      LOG_LEVEL                 = "info"
    })
  }

  depends_on = [aws_cloudwatch_log_group.history_worker, aws_iam_role_policy.history_worker]
}

# Scheduler delivery failures and Lambda execution failures both reach this DLQ.
resource "aws_lambda_function_event_invoke_config" "history_worker" {
  count                        = local.history_schedule_enabled ? 1 : 0
  function_name                = aws_lambda_function.history_worker[0].function_name
  maximum_event_age_in_seconds = 3600
  maximum_retry_attempts       = 0

  destination_config {
    on_failure {
      destination = aws_sqs_queue.history_dead_letter[0].arn
    }
  }
}

data "aws_iam_policy_document" "history_scheduler_assume" {
  count = local.history_schedule_enabled ? 1 : 0

  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["scheduler.amazonaws.com"]
    }
    condition {
      test     = "ArnEquals"
      variable = "aws:SourceArn"
      values   = ["arn:${data.aws_partition.current.partition}:scheduler:${var.aws_region}:${data.aws_caller_identity.current.account_id}:schedule/default/${local.function_name}-soccer-history-daily"]
    }
  }
}

resource "aws_iam_role" "history_scheduler" {
  count                = local.history_schedule_enabled ? 1 : 0
  name                 = "${local.function_name}-soccer-history-scheduler"
  assume_role_policy   = data.aws_iam_policy_document.history_scheduler_assume[0].json
  permissions_boundary = "arn:${data.aws_partition.current.partition}:iam::${data.aws_caller_identity.current.account_id}:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary"
}

data "aws_iam_policy_document" "history_scheduler" {
  count = local.history_schedule_enabled ? 1 : 0

  statement {
    actions   = ["lambda:InvokeFunction"]
    resources = [aws_lambda_function.history_worker[0].arn]
  }

  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.history_dead_letter[0].arn]
  }
}

resource "aws_iam_role_policy" "history_scheduler" {
  count  = local.history_schedule_enabled ? 1 : 0
  name   = "${local.function_name}-soccer-history-scheduler"
  role   = aws_iam_role.history_scheduler[0].id
  policy = data.aws_iam_policy_document.history_scheduler[0].json
}

resource "aws_scheduler_schedule" "history_daily" {
  count                        = local.history_schedule_enabled ? 1 : 0
  name                         = "${local.function_name}-soccer-history-daily"
  schedule_expression          = var.soccer_history_schedule_expression
  schedule_expression_timezone = "UTC"
  state                        = "ENABLED"

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = aws_lambda_function.history_worker[0].arn
    role_arn = aws_iam_role.history_scheduler[0].arn
    input    = jsonencode({ source = "portfolio.soccer-history.daily" })

    retry_policy {
      maximum_event_age_in_seconds = 3600
      maximum_retry_attempts       = 2
    }

    dead_letter_config {
      arn = aws_sqs_queue.history_dead_letter[0].arn
    }
  }

  depends_on = [aws_iam_role_policy.history_scheduler, aws_lambda_function_event_invoke_config.history_worker]
}

resource "aws_cloudwatch_log_metric_filter" "history_incomplete" {
  count          = local.history_schedule_enabled ? 1 : 0
  name           = "${local.function_name}-soccer-history-incomplete"
  pattern        = "{ $.msg = \"soccer_history_daily_incomplete\" }"
  log_group_name = aws_cloudwatch_log_group.history_worker[0].name

  metric_transformation {
    name      = "DailyIncomplete"
    namespace = "Portfolio/SoccerHistory"
    value     = "1"
  }
}

resource "aws_cloudwatch_log_metric_filter" "history_admission_rejected" {
  count          = local.history_collection_enabled ? 1 : 0
  name           = "${local.function_name}-soccer-history-admission-rejected"
  pattern        = "{ $.msg = \"soccer_history_admission_rejected\" }"
  log_group_name = aws_cloudwatch_log_group.lambda.name

  metric_transformation {
    name      = "AdmissionRejected"
    namespace = "Portfolio/SoccerHistory"
    value     = "1"
  }
}

resource "aws_cloudwatch_metric_alarm" "history_admission_rejected" {
  count               = local.history_collection_enabled ? 1 : 0
  alarm_name          = "${local.function_name}-soccer-history-admission-rejected"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "AdmissionRejected"
  namespace           = "Portfolio/SoccerHistory"
  period              = 300
  statistic           = "Sum"
  threshold           = 1
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_action_arns
}

resource "aws_cloudwatch_metric_alarm" "history_incomplete" {
  count               = local.history_schedule_enabled ? 1 : 0
  alarm_name          = "${local.function_name}-soccer-history-incomplete"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "DailyIncomplete"
  namespace           = "Portfolio/SoccerHistory"
  period              = 300
  statistic           = "Sum"
  threshold           = 1
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_action_arns
}

resource "aws_cloudwatch_metric_alarm" "history_worker_errors" {
  count               = local.history_schedule_enabled ? 1 : 0
  alarm_name          = "${local.function_name}-soccer-history-errors"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "Errors"
  namespace           = "AWS/Lambda"
  period              = 300
  statistic           = "Sum"
  threshold           = 1
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_action_arns

  dimensions = { FunctionName = aws_lambda_function.history_worker[0].function_name }
}

resource "aws_cloudwatch_metric_alarm" "history_dead_letter" {
  count               = local.history_schedule_enabled ? 1 : 0
  alarm_name          = "${local.function_name}-soccer-history-dead-letter"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 300
  statistic           = "Maximum"
  threshold           = 1
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_action_arns

  dimensions = { QueueName = aws_sqs_queue.history_dead_letter[0].name }
}
