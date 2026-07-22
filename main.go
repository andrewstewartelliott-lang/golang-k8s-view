package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	prometheusExporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	metricsProvider, metricsHandler, err := setupMetrics()
	if err != nil {
		log.Fatalf("failed to configure metrics: %v", err)
	}

	router := newRouter(metricsProvider, metricsHandler)

	log.Println("listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func setupMetrics() (*metric.MeterProvider, http.Handler, error) {
	registry := prometheus.NewRegistry()
	exporter, err := prometheusExporter.New(prometheusExporter.WithRegisterer(registry))
	if err != nil {
		return nil, nil, err
	}

	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return provider, handler, nil
}

func newRouter(provider *metric.MeterProvider, metricsHandler http.Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(otelgin.Middleware("golang-k8s-view"))

	router.GET("/metrics", func(c *gin.Context) {
		if metricsHandler != nil {
			metricsHandler.ServeHTTP(c.Writer, c.Request)
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "metrics handler unavailable"})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":   "Kubernetes resource listing service",
			"endpoints": []string{"/pods", "/services"},
		})
	})

	router.GET("/pods", func(c *gin.Context) {
		client, err := newKubeClient()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		pods, err := listPods(c.Request.Context(), client)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, pods)
	})

	router.GET("/services", func(c *gin.Context) {
		client, err := newKubeClient()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		services, err := listServices(c.Request.Context(), client)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, services)
	})

	return router
}

func newKubeClient() (kubernetes.Interface, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err == nil {
			return kubernetes.NewForConfig(config)
		}
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

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
