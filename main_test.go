package main

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

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
