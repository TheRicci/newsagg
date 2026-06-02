# newsagg

`newsagg` is a personal news aggregator that collects RSS articles, stores them,
enriches them with Gemini-generated context, and serves them through a Next.js
frontend.

The repository now has two architecture versions:

- **Local version**: k3d/Kubernetes with MongoDB, RabbitMQ, Go services, and
  a Next.js frontend.
- **Cloud version**: Vercel frontend plus an AWS free-tier-oriented serverless
  stack using Lambda, DynamoDB, SQS, EventBridge Scheduler, SSM, HCP Terraform,
  and GitHub Actions.

The local project remains the original self-hosted environment. The cloud
project lives in [`aws-free-tier/`](aws-free-tier/README.md).

## Architecture Overview

```mermaid
flowchart LR
  subgraph Local["Local k3d / Kubernetes"]
    LBrowser["Browser"] --> LIngress["Traefik Ingress"]
    LIngress --> LFrontend["Next.js frontend pod"]
    LFrontend --> LAPI["Go api-gateway pod"]
    LAPI --> LMongo["MongoDB"]
    LFetcher["Go fetcher pod"] --> LRSS["RSS feeds"]
    LFetcher --> LMongo
    LFetcher --> LRabbit["RabbitMQ articles.new"]
    LRabbit --> LEnrich["Go ai-enrich pod"]
    LEnrich --> LGemini["Gemini API"]
    LEnrich --> LMongo
  end

  subgraph Cloud["AWS + Vercel"]
    CBrowser["Browser"] --> CVercel["Vercel Next.js frontend"]
    CVercel --> CUrl["Lambda Function URL"]
    CUrl --> CAPI["Go API Lambda"]
    CAPI --> CDDB["DynamoDB articles + topic counts"]
    CScheduler["EventBridge Scheduler"] --> CFetcher["Go fetcher Lambda"]
    CFetcher --> CRSS["RSS feeds"]
    CFetcher --> CDDB
    CFetcher --> CSQS["SQS articles queue"]
    CSQS --> CEnrich["Go ai-enrich Lambda"]
    CEnrich --> CSSM["SSM Gemini parameter"]
    CEnrich --> CGemini["Gemini API"]
    CEnrich --> CDDB
  end
```

## Local Kubernetes Version

The local version is designed for a self-hosted development cluster. It keeps
state in MongoDB and uses RabbitMQ as the async boundary between article
ingestion and AI enrichment.

### Local Request Flow

```mermaid
sequenceDiagram
  participant User as Browser
  participant Ingress as Traefik Ingress
  participant Frontend as frontend pod
  participant API as api-gateway pod
  participant Mongo as MongoDB

  User->>Ingress: GET http://localhost
  Ingress->>Frontend: Serve Next.js UI
  Frontend->>API: /api/topics or /api/articles
  API->>Mongo: Query articles and topic counts
  Mongo-->>API: Results
  API-->>Frontend: JSON response
  Frontend-->>User: Render articles and enrichment
```

### Local Background Flow

```mermaid
sequenceDiagram
  participant Fetcher as fetcher pod
  participant RSS as RSS feeds
  participant Mongo as MongoDB
  participant Rabbit as RabbitMQ
  participant Enrich as ai-enrich pod
  participant Gemini as Gemini API

  loop Every 30 minutes
    Fetcher->>RSS: Read configured feeds
    Fetcher->>Mongo: Insert new articles
    Fetcher->>Rabbit: Publish article event to articles.new
  end

  Enrich->>Rabbit: Consume up to 10 articles
  Enrich->>Gemini: Request structured enrichment
  Gemini-->>Enrich: Summary, context, tags, confidence
  Enrich->>Mongo: Save enrichment fields
  Enrich->>Rabbit: Ack successful messages
```

### Local Services

| Service | Runtime | Role |
|---|---|---|
| `frontend` | Next.js / TypeScript | Browser UI, topic filtering, article cards, AI enrichment display |
| `api-gateway` | Go + chi | REST API for articles, topics, health checks |
| `fetcher` | Go | Polls RSS feeds, inserts new MongoDB articles, publishes RabbitMQ messages |
| `ai-enrich` | Go | Consumes RabbitMQ batches, calls Gemini, updates MongoDB |
| `mongo` | MongoDB 7 | Local article and enrichment store |
| `rabbitmq` | RabbitMQ 3 | Queue between fetcher and AI enrichment worker |

### API Endpoints

```text
GET /health
GET /topics
GET /articles
GET /articles?topic=ufology
GET /articles?topic=finance&limit=20&page=2
GET /articles/{id}
```

### MongoDB Article Shape

```json
{
  "_id": "ObjectId",
  "topic": "ufology | neuroscience | finance",
  "source": "feed source",
  "title": "Article headline",
  "url": "https://source.example/article",
  "summary": "Original RSS teaser text",
  "published_at": "2026-05-21T10:00:00Z",
  "fetched_at": "2026-05-21T10:30:00Z",
  "ai_summary": "One sentence summary from Gemini",
  "ai_key_points": ["point 1", "point 2"],
  "ai_context": "Background context paragraph",
  "ai_tags": ["tag1", "tag2"],
  "ai_confidence": "high | medium | low",
  "ai_enriched_at": "2026-05-21T10:35:00Z"
}
```

## Local Setup

### Prerequisites

- Docker
- k3d
- kubectl
- Go
- Node.js
- Gemini API key

### Create The k3d Cluster

```bash
k3d cluster create mycluster \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --port "30017:30017@server:0"
```

### Create Kubernetes Secrets

```bash
kubectl create secret generic mongo-secret \
  --from-literal=MONGO_URI=mongodb://mongo:27017

kubectl create secret generic rabbitmq-secret \
  --from-literal=RABBITMQ_USER=newsagg \
  --from-literal=RABBITMQ_PASS=newsagg123

kubectl create secret generic rabbitmq-secret-uri \
  --from-literal=RABBITMQ_URI=amqp://newsagg:newsagg123@rabbitmq:5672/

kubectl create secret generic gemini-secret \
  --from-literal=GEMINI_API_KEY=your-key-here
```

### Deploy Local Kubernetes Resources

```bash
kubectl apply -f k8s/infra/
kubectl apply -f k8s/fetcher/
kubectl apply -f k8s/api-gateway/
kubectl apply -f k8s/frontend/
kubectl apply -f k8s/ai-enrich/
```

### Access Local Services

| URL | Purpose |
|---|---|
| `http://localhost` | News aggregator UI |
| `http://rabbitmq.localhost` | RabbitMQ management UI |
| `mongodb://localhost:30017` | MongoDB access from Compass or `mongosh` |

For RabbitMQ hostname routing, add this to your hosts file:

```text
127.0.0.1 rabbitmq.localhost
```

## Build And Update A Local Service

```bash
cd services/<name>
docker build -t newsagg-<name>:vX.Y .
k3d image import newsagg-<name>:vX.Y -c mycluster
kubectl set image deployment/<name> <name>=newsagg-<name>:vX.Y
kubectl rollout status deployment/<name>
kubectl logs -f deployment/<name>
```

## Cloud Version

The cloud version keeps the same product idea but replaces local Kubernetes
infrastructure with serverless/free-tier-friendly services:

| Local | Cloud |
|---|---|
| k3d + Kubernetes | Terraform-managed AWS resources |
| frontend pod | Vercel Next.js deployment |
| Go api-gateway pod | Go API Lambda behind Lambda Function URL |
| MongoDB | DynamoDB tables |
| RabbitMQ queue | SQS queue and DLQ |
| fetcher pod | EventBridge-scheduled fetcher Lambda |
| ai-enrich pod | SQS-triggered enrich Lambda |
| Kubernetes secrets | SSM Parameter Store + GitHub/Vercel secrets |

Read the cloud documentation here:

```text
aws-free-tier/README.md
aws-free-tier/terraform/README.md
```

## Repository Structure

```text
newsagg-k8s/
  frontend/                 Local Kubernetes Next.js frontend
  services/
    api-gateway/            Local Go REST API
    fetcher/                Local Go RSS fetcher
    ai-enrich/              Local Go Gemini enrichment worker
  k8s/                      Local Kubernetes manifests
  aws-free-tier/
    frontend-vercel/        Vercel-ready Next.js frontend
    lambdas/                AWS Go Lambda services
    terraform/              AWS infrastructure as code
    scripts/                Cross-platform Lambda build helper
  .github/workflows/        CI/CD workflow for tests and AWS deploy
```

## Current Deployment Notes

- Local and cloud versions are intentionally separated.
- HCP Terraform stores the cloud Terraform state remotely.
- GitHub Actions validates the repo on pull requests and deploys AWS on pushes
  to `master`.
- Vercel deploys the cloud frontend separately from `aws-free-tier/frontend-vercel`.
- The cloud API URL is exported by Terraform as `api_function_url` and is used
  by Vercel as `API_GATEWAY_URL`.
