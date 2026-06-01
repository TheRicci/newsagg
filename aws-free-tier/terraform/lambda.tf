resource "null_resource" "build_lambda" {
  for_each = local.lambda_sources

  triggers = {
    go_mod_hash       = filesha256("${each.value}/go.mod")
    go_source_hash    = sha256(join("", [for f in fileset(each.value, "*.go") : filesha256("${each.value}/${f}")]))
    build_script_hash = filesha256(local.build_script)
  }

  provisioner "local-exec" {
    command = "\"${var.lambda_build_python_command}\" \"${local.build_script}\" --source-dir \"${each.value}\" --output-dir \"${local.build_root}/${each.key}\""
  }
}

data "archive_file" "lambda_zip" {
  for_each = local.lambda_sources

  type        = "zip"
  source_file = "${local.build_root}/${each.key}/bootstrap"
  output_path = "${local.build_root}/${each.key}.zip"

  depends_on = [null_resource.build_lambda]
}

resource "aws_cloudwatch_log_group" "api" {
  name              = "/aws/lambda/${local.name_prefix}-api"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "fetcher" {
  name              = "/aws/lambda/${local.name_prefix}-fetcher"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "enrich" {
  name              = "/aws/lambda/${local.name_prefix}-enrich"
  retention_in_days = 7
}

resource "aws_lambda_function" "api" {
  function_name = "${local.name_prefix}-api"
  role          = aws_iam_role.lambda.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["arm64"]
  memory_size   = var.api_lambda_memory_mb
  timeout       = 15

  filename         = data.archive_file.lambda_zip["api"].output_path
  source_code_hash = data.archive_file.lambda_zip["api"].output_base64sha256

  environment {
    variables = {
      ARTICLES_TABLE        = aws_dynamodb_table.articles.name
      TOPIC_COUNTS_TABLE    = aws_dynamodb_table.topic_counts.name
      ACCESS_CONTROL_ORIGIN = join(",", var.allowed_cors_origins)
    }
  }

  depends_on = [
    aws_cloudwatch_log_group.api,
    aws_iam_role_policy.lambda
  ]
}

resource "aws_lambda_function_url" "api" {
  function_name      = aws_lambda_function.api.function_name
  authorization_type = "NONE"

  cors {
    allow_credentials = false
    allow_headers     = ["content-type"]
    allow_methods     = ["GET"]
    allow_origins     = var.allowed_cors_origins
    max_age           = 3600
  }
}

resource "aws_lambda_function" "fetcher" {
  function_name = "${local.name_prefix}-fetcher"
  role          = aws_iam_role.lambda.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["arm64"]
  memory_size   = var.worker_lambda_memory_mb
  timeout       = 60

  filename         = data.archive_file.lambda_zip["fetcher"].output_path
  source_code_hash = data.archive_file.lambda_zip["fetcher"].output_base64sha256

  reserved_concurrent_executions = var.worker_reserved_concurrency

  environment {
    variables = {
      ARTICLES_TABLE     = aws_dynamodb_table.articles.name
      TOPIC_COUNTS_TABLE = aws_dynamodb_table.topic_counts.name
      ARTICLES_QUEUE_URL = aws_sqs_queue.articles.url
    }
  }

  depends_on = [
    aws_cloudwatch_log_group.fetcher,
    aws_iam_role_policy.lambda
  ]
}

resource "aws_lambda_function" "enrich" {
  function_name = "${local.name_prefix}-enrich"
  role          = aws_iam_role.lambda.arn
  runtime       = "provided.al2023"
  handler       = "bootstrap"
  architectures = ["arm64"]
  memory_size   = var.worker_lambda_memory_mb
  timeout       = 120

  filename         = data.archive_file.lambda_zip["enrich"].output_path
  source_code_hash = data.archive_file.lambda_zip["enrich"].output_base64sha256

  reserved_concurrent_executions = var.worker_reserved_concurrency

  environment {
    variables = {
      ARTICLES_TABLE           = aws_dynamodb_table.articles.name
      GEMINI_API_KEY_PARAMETER = local.gemini_parameter_name
      GEMINI_MODEL             = var.gemini_model
    }
  }

  depends_on = [
    aws_cloudwatch_log_group.enrich,
    aws_iam_role_policy.lambda
  ]
}

resource "aws_lambda_event_source_mapping" "enrich_from_sqs" {
  event_source_arn                   = aws_sqs_queue.articles.arn
  function_name                      = aws_lambda_function.enrich.arn
  batch_size                         = 10
  maximum_batching_window_in_seconds = 30
  enabled                            = var.enable_ai_enrichment
  function_response_types            = ["ReportBatchItemFailures"]
}

resource "aws_lambda_permission" "api_url_public" {
  statement_id           = "FunctionURLAllowPublicAccess"
  action                 = "lambda:InvokeFunctionUrl"
  function_name          = aws_lambda_function.api.function_name
  principal              = "*"
  function_url_auth_type = "NONE"
}

resource "aws_lambda_permission" "api_url_invoke_function" {
  statement_id             = "FunctionURLAllowInvokeFunction"
  action                   = "lambda:InvokeFunction"
  function_name            = aws_lambda_function.api.function_name
  principal                = "*"
  invoked_via_function_url = true
}
