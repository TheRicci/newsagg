resource "aws_scheduler_schedule" "fetcher" {
  name                         = "${local.name_prefix}-fetcher"
  schedule_expression          = var.fetch_schedule_expression
  state                        = var.enable_fetch_schedule ? "ENABLED" : "DISABLED"
  schedule_expression_timezone = "UTC"

  flexible_time_window {
    mode = "OFF"
  }

  target {
    arn      = aws_lambda_function.fetcher.arn
    role_arn = aws_iam_role.scheduler.arn
    input    = jsonencode({ source = "eventbridge-scheduler" })

    retry_policy {
      maximum_event_age_in_seconds = 3600
      maximum_retry_attempts       = 2
    }
  }
}

