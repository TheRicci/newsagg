# Stage 1 — build the binary
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o fetcher .

# Stage 2 — minimal runtime image
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/fetcher .
EXPOSE 8080
CMD ["./fetcher"]
