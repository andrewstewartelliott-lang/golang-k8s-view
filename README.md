# golang-k8s-view

A small Go service that exposes a Gin API for listing Kubernetes pods across all namespaces.

## What it does

The app starts a web server and serves:

- `GET /pods` - returns a JSON array of pod summaries from the connected Kubernetes cluster

Each pod entry includes:
- namespace
- name
- status
- readiness (`ready/total`)
- node name (when available)

## How it works

The application:
- creates a Kubernetes client from either:
  - a kubeconfig file via `KUBECONFIG`, or
  - in-cluster configuration when running inside Kubernetes
- lists pods using the Kubernetes core API
- sorts the results by namespace and pod name before returning them

## Run locally

1. Make sure your Kubernetes context is configured:
   ```bash
   kubectl config current-context
   ```

2. Run the app:
   ```bash
   go run .
   ```

3. Open the endpoint:
   ```bash
   curl http://localhost:8080/pods
   ```

## Run inside Kubernetes

When deployed into a cluster, the app will use in-cluster configuration automatically.

Make sure the pod service account has permission to list pods, for example via a Role or ClusterRole such as:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pod-viewer
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
```

## Development

Run the tests:

```bash
go test -v
```
