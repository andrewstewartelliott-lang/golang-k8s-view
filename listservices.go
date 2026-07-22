package main

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func listServices(ctx context.Context, client kubernetes.Interface) ([]serviceSummary, error) {
	items, err := client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	serviceSummaries := make([]serviceSummary, 0, len(items.Items))
	for _, svc := range items.Items {
		ports := make([]string, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
		}

		serviceSummaries = append(serviceSummaries, serviceSummary{
			Namespace: svc.Namespace,
			Name:      svc.Name,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			Ports:     ports,
		})
	}

	sort.Slice(serviceSummaries, func(i, j int) bool {
		if serviceSummaries[i].Namespace != serviceSummaries[j].Namespace {
			return serviceSummaries[i].Namespace < serviceSummaries[j].Namespace
		}
		return serviceSummaries[i].Name < serviceSummaries[j].Name
	})

	return serviceSummaries, nil
}
