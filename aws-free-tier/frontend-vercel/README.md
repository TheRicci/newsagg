# newsagg Vercel frontend

This is the Vercel frontend for:

```text
Vercel Next.js frontend
  -> AWS Lambda Function URL API
  -> DynamoDB articles + topic counters
```

Deploy this folder as the Vercel project root.

Set the Vercel environment variable:

```text
API_GATEWAY_URL=https://your-lambda-function-url.lambda-url.us-east-1.on.aws
```

Do not include a trailing slash.

The app calls `/api/topics` and `/api/articles`. `next.config.ts` rewrites
those requests to the Lambda Function URL.
