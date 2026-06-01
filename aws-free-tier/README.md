# newsagg cloud migration

This folder contains the AWS + Vercel version of `newsagg`. It is isolated from
the existing local k3d/Kubernetes project.

## Architecture

```text
Vercel Next.js frontend
  -> AWS Lambda Function URL API
  -> DynamoDB articles + topic counters

EventBridge Scheduler
  -> fetcher Lambda
  -> DynamoDB
  -> SQS articles queue
  -> ai-enrich Lambda
  -> Gemini API
  -> DynamoDB
```

## Why This Design

- Vercel hosts the Next.js UI on a free personal plan.
- Lambda Function URL exposes the Go API without API Gateway.
- DynamoDB replaces MongoDB with an AWS-native free-tier-friendly database.
- SQS replaces RabbitMQ for async article enrichment.
- EventBridge Scheduler replaces a long-running fetcher container.
- Lambdas stay outside a VPC to avoid NAT Gateway costs.

## Folders

```text
frontend-vercel/   Next.js app to deploy on Vercel
lambdas/api/       Public read API: health, topics, articles
lambdas/fetcher/   Scheduled RSS ingestion function
lambdas/enrich/    SQS-triggered Gemini enrichment function
terraform/         AWS infrastructure
scripts/           Cross-platform Python Lambda build helper
```

## First Deploy

Start with only the frontend/API/database path. Keep background work disabled:

```hcl
enable_fetch_schedule = false
enable_ai_enrichment  = false
gemini_api_key        = ""
```

Then provision AWS:

```powershell
cd aws-free-tier\terraform
Copy-Item terraform.tfvars.example terraform.tfvars
terraform init
terraform validate
terraform plan
terraform apply
```

Use the `api_function_url` output as the Vercel environment variable:

```text
API_GATEWAY_URL=https://your-lambda-url.lambda-url.us-east-1.on.aws
```

Deploy `frontend-vercel/` as the Vercel project root.

## GitHub Actions AWS Deploy

This repo includes a free-tier-friendly GitHub Actions workflow at
`.github/workflows/aws-free-tier.yml`. On pull requests to `master`, it runs Go
tests, Next.js lint/build checks, and Terraform validation. On pushes to
`master`, it runs the same checks and then applies the AWS Terraform stack.

Configure these repository secrets before relying on automatic deploys:

```text
TF_TOKEN_APP_TERRAFORM_IO  HCP Terraform token for remote state
AWS_ROLE_TO_ASSUME     Recommended: IAM role ARN for GitHub OIDC deploys
GEMINI_API_KEY         Gemini key used to keep the SSM parameter managed
```

If you are not using OIDC yet, set these repository secrets instead of
`AWS_ROLE_TO_ASSUME`:

```text
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
```

Optional repository variables:

```text
AWS_REGION             Defaults to us-east-1
ALLOWED_CORS_ORIGINS   Defaults to ["*"]
```

Required repository variables:

```text
ENABLE_FETCH_SCHEDULE  true or false
ENABLE_AI_ENRICHMENT   true or false
```

These flags are required so the workflow fails safely instead of accidentally
turning running background work off. The workflow uses HCP Terraform remote
state through the `cloud` block in `terraform/versions.tf`, so create the HCP
Terraform workspace first and keep it in CLI-driven, local execution mode.

If you already ran `terraform apply` from your laptop, migrate local state to
HCP Terraform before the first GitHub Actions deploy:

```powershell
cd aws-free-tier\terraform
terraform login
terraform init
terraform plan
```

## Enable Background Work

After the frontend can read from the Lambda API, enable RSS ingestion:

```hcl
enable_fetch_schedule = true
enable_ai_enrichment  = false
```

After articles appear, configure Gemini and enable enrichment:

```hcl
gemini_api_key       = "your-gemini-key"
enable_ai_enrichment = true
```

For stricter secret handling, create the Gemini key manually in SSM Parameter
Store instead of storing it in Terraform state.

## Terraform

See `terraform/README.md` for a file-by-file explanation of the AWS stack.
