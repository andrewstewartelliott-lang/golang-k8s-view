package main

import (
	"log"
)

type podSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Node      string `json:"node,omitempty"`
}

type serviceSummary struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	ClusterIP string   `json:"clusterIP"`
	Ports     []string `json:"ports,omitempty"`
}

func main() {
	_, metricsHandler, err := setupMetrics()
	if err != nil {
		log.Fatalf("failed to configure metrics: %v", err)
	}

	router := newRouter(metricsHandler)

	log.Println("listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
