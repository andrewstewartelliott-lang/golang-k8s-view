package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewRouterRespondsToRoot(t *testing.T) {
	router := newRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !strings.Contains(w.Body.String(), "Kubernetes resource listing service") {
		t.Fatalf("expected response body to mention service description, got %q", w.Body.String())
	}
}

func TestNewRouterRespondsToMetrics(t *testing.T) {
	router := newRouter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("test_metric 1"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if !strings.Contains(w.Body.String(), "test_metric 1") {
		t.Fatalf("expected metrics body to contain sample metric, got %q", w.Body.String())
	}
}

func TestListPodsSummarizesPodState(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "nginx"}},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{Name: "nginx", Ready: true}},
		},
	})

	pods, err := listPods(context.Background(), client)
	if err != nil {
		t.Fatalf("listPods returned error: %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("expected 1 pod summary, got %d", len(pods))
	}

	got := pods[0]
	if got.Namespace != "default" {
		t.Fatalf("expected namespace default, got %q", got.Namespace)
	}
	if got.Name != "nginx" {
		t.Fatalf("expected pod name nginx, got %q", got.Name)
	}
	if got.Status != "running" {
		t.Fatalf("expected status running, got %q", got.Status)
	}
	if got.Ready != "1/1" {
		t.Fatalf("expected ready state 1/1, got %q", got.Ready)
	}
}

func TestListPodsHandlesUnknownPhaseAndNoContainers(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "kube-system"},
		Spec:       corev1.PodSpec{},
		Status:     corev1.PodStatus{},
	})

	pods, err := listPods(context.Background(), client)
	if err != nil {
		t.Fatalf("listPods returned error: %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("expected 1 pod summary, got %d", len(pods))
	}

	got := pods[0]
	if got.Status != "unknown" {
		t.Fatalf("expected status unknown, got %q", got.Status)
	}
	if got.Ready != "0/0" {
		t.Fatalf("expected ready state 0/0, got %q", got.Ready)
	}
}

func TestListPodsSortsByNamespaceAndName(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "zeta", Namespace: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "kube-system"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "beta", Namespace: "default"}},
	)

	pods, err := listPods(context.Background(), client)
	if err != nil {
		t.Fatalf("listPods returned error: %v", err)
	}

	if len(pods) != 3 {
		t.Fatalf("expected 3 pod summaries, got %d", len(pods))
	}

	if pods[0].Namespace != "default" || pods[0].Name != "beta" {
		t.Fatalf("expected first pod to be beta in default namespace, got %#v", pods[0])
	}
	if pods[1].Namespace != "default" || pods[1].Name != "zeta" {
		t.Fatalf("expected second pod to be zeta in default namespace, got %#v", pods[1])
	}
	if pods[2].Namespace != "kube-system" || pods[2].Name != "alpha" {
		t.Fatalf("expected third pod to be alpha in kube-system namespace, got %#v", pods[2])
	}
}

func TestListServicesSummarizesServiceState(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
			Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	})

	services, err := listServices(context.Background(), client)
	if err != nil {
		t.Fatalf("listServices returned error: %v", err)
	}

	if len(services) != 1 {
		t.Fatalf("expected 1 service summary, got %d", len(services))
	}

	got := services[0]
	if got.Namespace != "default" {
		t.Fatalf("expected namespace default, got %q", got.Namespace)
	}
	if got.Name != "api" {
		t.Fatalf("expected service name api, got %q", got.Name)
	}
	if got.Type != string(corev1.ServiceTypeClusterIP) {
		t.Fatalf("expected service type %q, got %q", corev1.ServiceTypeClusterIP, got.Type)
	}
	if got.ClusterIP != "10.0.0.1" {
		t.Fatalf("expected cluster IP 10.0.0.1, got %q", got.ClusterIP)
	}
	if len(got.Ports) != 1 || got.Ports[0] != "80/TCP" {
		t.Fatalf("expected ports [80/TCP], got %v", got.Ports)
	}
}

func TestListServicesSortsByNamespaceAndName(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "zeta", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "alpha", Namespace: "kube-system"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "beta", Namespace: "default"}},
	)

	services, err := listServices(context.Background(), client)
	if err != nil {
		t.Fatalf("listServices returned error: %v", err)
	}

	if len(services) != 3 {
		t.Fatalf("expected 3 service summaries, got %d", len(services))
	}

	if services[0].Namespace != "default" || services[0].Name != "beta" {
		t.Fatalf("expected first service to be beta in default namespace, got %#v", services[0])
	}
	if services[1].Namespace != "default" || services[1].Name != "zeta" {
		t.Fatalf("expected second service to be zeta in default namespace, got %#v", services[1])
	}
	if services[2].Namespace != "kube-system" || services[2].Name != "alpha" {
		t.Fatalf("expected third service to be alpha in kube-system namespace, got %#v", services[2])
	}
}
