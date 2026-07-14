package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePodLister struct {
	pods []podSummary
	err  error
}

func (f fakePodLister) ListPods(ctx context.Context) ([]podSummary, error) {
	return f.pods, f.err
}

type fakeRBACEnforcer struct {
	calls []struct {
		serviceAccountName      string
		serviceAccountNamespace string
	}
	err error
}

func (f *fakeRBACEnforcer) EnsureViewClusterRole(ctx context.Context, serviceAccountName, serviceAccountNamespace string) error {
	f.calls = append(f.calls, struct {
		serviceAccountName      string
		serviceAccountNamespace string
	}{serviceAccountName: serviceAccountName, serviceAccountNamespace: serviceAccountNamespace})
	return f.err
}

func TestPodsEndpointListsPods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := buildRouter(fakePodLister{pods: []podSummary{{Namespace: "default", Name: "nginx", Status: "running", Ready: "1/1"}}}, nil)

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var got []podSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 pod summary, got %d", len(got))
	}

	if got[0].Name != "nginx" {
		t.Fatalf("expected pod name %q, got %q", "nginx", got[0].Name)
	}
}

func TestEnsureViewEndpointCreatesClusterRoleBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeRBACEnforcer{}
	router := buildRouter(fakePodLister{}, fake)

	payload := `{"serviceAccountName":"demo","serviceAccountNamespace":"kube-system"}`
	req := httptest.NewRequest(http.MethodPost, "/rbac/ensure-view", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 RBAC call, got %d", len(fake.calls))
	}

	if fake.calls[0].serviceAccountName != "demo" {
		t.Fatalf("expected service account name %q, got %q", "demo", fake.calls[0].serviceAccountName)
	}

	if fake.calls[0].serviceAccountNamespace != "kube-system" {
		t.Fatalf("expected service account namespace %q, got %q", "kube-system", fake.calls[0].serviceAccountNamespace)
	}
}
