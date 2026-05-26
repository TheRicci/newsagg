# newsagg

A self-hosted personal news aggregator that fetches articles from RSS feeds, stores them in MongoDB, enriches them with AI-generated summaries via the Gemini API, and serves them through a clean Next.js frontend — all running inside a local Kubernetes cluster.

---

## Architecture

```
Browser
  │
  │  http://localhost
  ▼
k3d LoadBalancer (nginx)
  │
  ▼
Traefik Ingress Controller
  │
  ├── localhost          → frontend (Next.js)
  └── rabbitmq.localhost → RabbitMQ Management UI
          │
          ▼
    frontend pod
          │  /api/* proxy
          ▼
    api-gateway pod (Go)
          │
          ▼
    MongoDB


Independently running:

fetcher pod (Go)
  ├── polls 6 RSS feeds every 30 minutes
  ├── saves new articles to MongoDB
  └── publishes new article events to RabbitMQ

ai-enrich pod (Go)
  ├── consumes from RabbitMQ in batches of 10
  ├── calls Gemini API to enrich each article
  │     (summary, key points, context, tags, confidence)
  └── saves enrichment back to MongoDB
```

---

## Services

| Service | Language | Port | Description |
|---|---|---|---|
| frontend | Next.js / TypeScript | 3000 | UI — article list, topic filter, AI enrichment cards |
| api-gateway | Go | 8081 | REST API — serves articles and topics from MongoDB |
| fetcher | Go | 8080 | RSS poller — fetches 6 feeds every 30 minutes |
| ai-enrich | Go | 8082 | AI enricher — batches articles through Gemini API |
| mongo | MongoDB 7 | 27017 | Primary data store |
| rabbitmq | RabbitMQ 3 | 5672 / 15672 | Message queue between fetcher and ai-enrich |

---

## MongoDB Document Structure

```json
{
  "_id": "ObjectId",
  "topic": "ufology | neuroscience | finance",
  "source": "openminds | newsnation | ...",
  "title": "Article headline",
  "url": "https://source.com/article",
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

---

## API Endpoints

```
GET /health                              → service health check
GET /topics                             → list topics with article counts
GET /articles                           → paginated article list
GET /articles?topic=ufology             → filter by topic
GET /articles?topic=finance&limit=20&page=2
GET /articles/:id                       → single article by ID
```

---

## Infrastructure

### Kubernetes Resources

```
k8s/
├── infra/
│   ├── mongo-deployment.yaml    → MongoDB + PVC + Service + NodePort
│   └── rabbitmq.yaml            → RabbitMQ + PVC + Service
├── fetcher/
│   ├── deployment.yaml
│   └── service.yaml
├── api-gateway/
│   └── deployment.yaml          → Deployment + Service
├── frontend/
│   ├── deployment.yaml          → Deployment + Service
│   └── ingress.yaml             → Traefik Ingress rules
└── ai-enrich/
    └── deployment.yaml          → Deployment + Service
```

### Kubernetes Secrets Required

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

---

## Local Development Setup

### Prerequisites

- Docker
- k3d
- kubectl
- Go 1.26+
- Node.js 22+

### Create the cluster

```bash
k3d cluster create mycluster \
  --port "80:80@loadbalancer" \
  --port "443:443@loadbalancer" \
  --port "30017:30017@server:0"
```

### Import images

```bash
k3d image import \
  newsagg-fetcher:v0.4 \
  newsagg-api-gateway:v0.2 \
  newsagg-frontend:v0.3 \
  newsagg-ai-enrich:v0.2 \
  -c mycluster
```

### Create secrets (see above)

### Deploy

```bash
kubectl apply -f k8s/infra/
kubectl apply -f k8s/fetcher/
kubectl apply -f k8s/api-gateway/
kubectl apply -f k8s/frontend/
kubectl apply -f k8s/ai-enrich/
kubectl scale deployment/ai-enrich --replicas=1
```

### Access

| URL | What |
|---|---|
| http://localhost | News aggregator UI |
| http://rabbitmq.localhost | RabbitMQ management UI |
| mongodb://localhost:30017 | MongoDB (Compass or mongosh) |

Add to `/etc/hosts` for RabbitMQ hostname routing:
```
127.0.0.1 rabbitmq.localhost
```

---

## Building a Service

```bash
# 1. build
cd services/<name>
docker build -t newsagg-<name>:vX.Y .

# 2. import into cluster
k3d image import newsagg-<name>:vX.Y -c mycluster

# 3. deploy
kubectl set image deployment/<name> <name>=newsagg-<name>:vX.Y
kubectl rollout status deployment/<name>

# 4. check logs
kubectl logs -f deployment/<name>
```

---

## Stopping and Resuming

```bash
# stop (saves state)
k3d cluster stop mycluster

# resume
k3d cluster start mycluster
kubectl get pods
```

---

## AI Enrichment

The `ai-enrich` service consumes articles from the `articles.new` RabbitMQ queue in batches:

- **Batch size**: 10 articles per Gemini API call
- **Batch timeout**: 30 seconds (sends partial batch if queue is slow)
- **Rate limit**: 10 RPM (Gemini 2.5 Flash-Lite free tier)
- **Model**: `gemini-2.5-flash-lite` (15 RPM, 1000 RPD free tier)
- **Retry**: exponential backoff starting at 30s, capped at 10 minutes
- **Ack strategy**: manual — messages only acknowledged after successful save

Gemini enriches each article by fetching the URL when accessible, returning structured JSON with summary, key points, context, tags, and a confidence level (high/medium/low).

---

## Project Structure

```
newsagg/
├── services/
│   ├── fetcher/         Go — RSS poller
│   ├── api-gateway/     Go — REST API
│   └── ai-enrich/       Go — Gemini enrichment worker
├── frontend/            Next.js — UI
├── k8s/                 Kubernetes manifests
└── README.md
```
