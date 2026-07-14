# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download 
COPY . .
ARG CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    go build  -o ./golang-k8s-view

# Final minimal stage
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/golang-k8s-view .
EXPOSE 8080
CMD ["./golang-k8s-view"]