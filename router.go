package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	prometheusExporter "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

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

func newRouter(metricsHandler http.Handler) *gin.Engine {
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
