package main

import (
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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
