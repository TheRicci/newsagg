output "api_function_url" {
  description = "Public Lambda Function URL. Use this as Vercel API_GATEWAY_URL without trailing slash."
  value       = trimsuffix(aws_lambda_function_url.api.function_url, "/")
}

output "articles_table_name" {
  value = aws_dynamodb_table.articles.name
}

output "topic_counts_table_name" {
  value = aws_dynamodb_table.topic_counts.name
}

output "articles_queue_url" {
  value = aws_sqs_queue.articles.url
}

output "articles_dlq_url" {
  value = aws_sqs_queue.articles_dlq.url
}

output "gemini_parameter_name" {
  value = local.gemini_parameter_name
}

