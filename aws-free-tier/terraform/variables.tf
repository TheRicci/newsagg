variable "aws_region" {
  description = "AWS region for the serverless deployment."
  type        = string
  default     = "us-east-1"
}

variable "project_name" {
  description = "Short project name used in AWS resource names."
  type        = string
  default     = "newsagg"

  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.project_name))
    error_message = "Use lowercase letters, numbers, and hyphens only."
  }
}

variable "environment" {
  description = "Environment suffix used in AWS resource names."
  type        = string
  default     = "dev"

  validation {
    condition     = can(regex("^[a-z0-9-]+$", var.environment))
    error_message = "Use lowercase letters, numbers, and hyphens only."
  }
}

variable "allowed_cors_origins" {
  description = "Origins allowed to call the Lambda Function URL directly."
  type        = list(string)
  default     = ["*"]
}

variable "fetch_schedule_expression" {
  description = "EventBridge Scheduler expression for RSS fetching."
  type        = string
  default     = "rate(30 minutes)"
}

variable "enable_fetch_schedule" {
  description = "Enable the scheduled RSS fetcher."
  type        = bool
  default     = false
}

variable "enable_ai_enrichment" {
  description = "Enable SQS events to invoke the AI enrichment Lambda."
  type        = bool
  default     = false
}

variable "gemini_api_key" {
  description = "Gemini API key. If empty, Terraform will not create the SSM parameter."
  type        = string
  sensitive   = true
  default     = ""
}

variable "gemini_model" {
  description = "Gemini model used by the enrichment Lambda."
  type        = string
  default     = "gemini-2.5-flash-lite"
}

variable "api_lambda_memory_mb" {
  description = "Memory for the API Lambda."
  type        = number
  default     = 128
}

variable "worker_lambda_memory_mb" {
  description = "Memory for fetcher and enrichment Lambdas."
  type        = number
  default     = 256
}

variable "worker_reserved_concurrency" {
  description = "Optional reserved concurrency for fetcher and enrichment Lambdas. Leave null for AWS accounts with low concurrency quotas."
  type        = number
  default     = null
}
