# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download 
COPY . .
ARG CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    go build  -o ./golang-kubernetes-api

# Final minimal stage
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/golang-kubernetes-api .
EXPOSE 8080
CMD ["./golang-kubernetes-api"]