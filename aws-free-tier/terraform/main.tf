locals {
  name_prefix = "${var.project_name}-${var.environment}"

  api_source     = abspath("${path.module}/../lambdas/api")
  fetcher_source = abspath("${path.module}/../lambdas/fetcher")
  enrich_source  = abspath("${path.module}/../lambdas/enrich")
  build_root     = abspath("${path.module}/../build")
  build_script   = abspath("${path.module}/../scripts/build-lambda.py")

  lambda_sources = {
    api     = local.api_source
    fetcher = local.fetcher_source
    enrich  = local.enrich_source
  }

  gemini_parameter_name = "/${local.name_prefix}/gemini-api-key"
}

data "aws_caller_identity" "current" {}

resource "aws_ssm_parameter" "gemini_api_key" {
  count = var.gemini_api_key == "" ? 0 : 1

  name  = local.gemini_parameter_name
  type  = "SecureString"
  value = var.gemini_api_key
}
