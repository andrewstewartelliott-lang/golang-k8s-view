package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type fakePodLister struct {
	pods []podSummary
	err  error
}

func (f fakePodLister) ListPods(ctx context.Context) ([]podSummary, error) {
	return f.pods, f.err
}

type flakyPodLister struct {
	calls int
	pods  []podSummary
	err   error
}

func (f *flakyPodLister) ListPods(ctx context.Context) ([]podSummary, error) {
	f.calls++
	if f.calls == 1 {
		return nil, fmt.Errorf("forbidden: cannot access pods resource")
	}
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

func TestPodsEndpointRetriesAfterEnsuringViewAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	lister := &flakyPodLister{pods: []podSummary{{Namespace: "default", Name: "nginx", Status: "running", Ready: "1/1"}}}
	fakeEnforcer := &fakeRBACEnforcer{}
	router := buildRouter(lister, fakeEnforcer)

	req := httptest.NewRequest(http.MethodGet, "/pods", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if lister.calls != 2 {
		t.Fatalf("expected 2 pod list attempts, got %d", lister.calls)
	}

	if len(fakeEnforcer.calls) != 1 {
		t.Fatalf("expected 1 RBAC update call, got %d", len(fakeEnforcer.calls))
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

func TestNewKubeClientUsesDefaultKubeconfigWhenNoExplicitPathIsSet(t *testing.T) {
	originalBuildConfigFromFlags := buildConfigFromFlags
	originalInClusterConfig := inClusterConfig
	originalNewKubeClientFromConfig := newKubeClientFromConfig

	t.Cleanup(func() {
		buildConfigFromFlags = originalBuildConfigFromFlags
		inClusterConfig = originalInClusterConfig
		newKubeClientFromConfig = originalNewKubeClientFromConfig
	})

	t.Setenv("KUBECONFIG", "")

	buildConfigFromFlags = func(masterURL, kubeconfigPath string) (*rest.Config, error) {
		if kubeconfigPath == "" {
			return &rest.Config{Host: "https://default-kubeconfig"}, nil
		}
		return nil, fmt.Errorf("unexpected kubeconfig path %q", kubeconfigPath)
	}

	inClusterConfig = func() (*rest.Config, error) {
		return nil, fmt.Errorf("in-cluster unavailable")
	}

	newKubeClientFromConfig = func(cfg *rest.Config) (kubernetes.Interface, error) {
		if cfg == nil || cfg.Host != "https://default-kubeconfig" {
			t.Fatalf("expected default kubeconfig host, got %v", cfg)
		}
		return kubernetes.NewForConfig(cfg)
	}

	client, err := newKubeClient()
	if err != nil {
		t.Fatalf("expected client creation to succeed, got %v", err)
	}
	if client == nil {
		t.Fatal("expected a kubernetes client")
	}
}

func TestNewKubeClientFallsBackToInClusterConfig(t *testing.T) {
	originalBuildConfigFromFlags := buildConfigFromFlags
	originalInClusterConfig := inClusterConfig
	originalNewKubeClientFromConfig := newKubeClientFromConfig

	t.Cleanup(func() {
		buildConfigFromFlags = originalBuildConfigFromFlags
		inClusterConfig = originalInClusterConfig
		newKubeClientFromConfig = originalNewKubeClientFromConfig
	})

	t.Setenv("KUBECONFIG", "/tmp/does-not-exist")

	buildConfigFromFlags = func(masterURL, kubeconfigPath string) (*rest.Config, error) {
		return nil, fmt.Errorf("kubeconfig load failed")
	}

	inClusterConfig = func() (*rest.Config, error) {
		return &rest.Config{Host: "https://10.0.0.1"}, nil
	}

	newKubeClientFromConfig = func(cfg *rest.Config) (kubernetes.Interface, error) {
		if cfg == nil || cfg.Host != "https://10.0.0.1" {
			t.Fatalf("expected in-cluster config host, got %v", cfg)
		}
		return kubernetes.NewForConfig(cfg)
	}

	client, err := newKubeClient()
	if err != nil {
		t.Fatalf("expected client creation to succeed, got %v", err)
	}
	if client == nil {
		t.Fatal("expected a kubernetes client")
	}
}
