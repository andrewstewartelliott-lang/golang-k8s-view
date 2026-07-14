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

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

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

	log.Println("listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
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
