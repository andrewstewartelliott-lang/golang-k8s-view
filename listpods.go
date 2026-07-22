package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func listPods(ctx context.Context, client kubernetes.Interface) ([]podSummary, error) {
	items, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podSummaries := make([]podSummary, 0, len(items.Items))
	for _, pod := range items.Items {
		readyCount := 0
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.Ready {
				readyCount++
			}
		}

		ready := fmt.Sprintf("0/%d", len(pod.Spec.Containers))
		if len(pod.Spec.Containers) > 0 {
			ready = fmt.Sprintf("%d/%d", readyCount, len(pod.Spec.Containers))
		}

		status := strings.ToLower(string(pod.Status.Phase))
		if status == "" {
			status = "unknown"
		}

		podSummaries = append(podSummaries, podSummary{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			Status:    status,
			Ready:     ready,
			Node:      pod.Spec.NodeName,
		})
	}

	sort.Slice(podSummaries, func(i, j int) bool {
		if podSummaries[i].Namespace != podSummaries[j].Namespace {
			return podSummaries[i].Namespace < podSummaries[j].Namespace
		}
		return podSummaries[i].Name < podSummaries[j].Name
	})

	return podSummaries, nil
}
