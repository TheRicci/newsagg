FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ai-enrich .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/ai-enrich .
EXPOSE 8082
CMD ["./ai-enrich"]
