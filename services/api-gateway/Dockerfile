FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api-gateway .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/api-gateway .
EXPOSE 8081
CMD ["./api-gateway"]
