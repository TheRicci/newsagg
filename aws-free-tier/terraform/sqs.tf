resource "aws_sqs_queue" "articles_dlq" {
  name                      = "${local.name_prefix}-articles-dlq"
  message_retention_seconds = 1209600
  sqs_managed_sse_enabled   = true
}

resource "aws_sqs_queue" "articles" {
  name                       = "${local.name_prefix}-articles"
  message_retention_seconds  = 345600
  receive_wait_time_seconds  = 20
  visibility_timeout_seconds = 180
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.articles_dlq.arn
    maxReceiveCount     = 3
  })
}

