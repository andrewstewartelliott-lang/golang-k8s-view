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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	buildConfigFromFlags    = clientcmd.BuildConfigFromFlags
	inClusterConfig         = rest.InClusterConfig
	newKubeClientFromConfig = func(cfg *rest.Config) (kubernetes.Interface, error) {
		return kubernetes.NewForConfig(cfg)
	}
)

type podSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Ready     string `json:"ready"`
	Node      string `json:"node,omitempty"`
}

type podLister interface {
	ListPods(ctx context.Context) ([]podSummary, error)
}

type kubePodLister struct {
	client kubernetes.Interface
}

func (l kubePodLister) ListPods(ctx context.Context) ([]podSummary, error) {
	items, err := l.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podSummaries := make([]podSummary, 0, len(items.Items))
	for _, pod := range items.Items {
		ready := fmt.Sprintf("0/%d", len(pod.Spec.Containers))
		readyCount := 0
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.Ready {
				readyCount++
			}
		}
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

type rbacEnforcer interface {
	EnsureViewClusterRole(ctx context.Context, serviceAccountName, serviceAccountNamespace string) error
}

type kubeRBACEnforcer struct {
	client kubernetes.Interface
}

func (e kubeRBACEnforcer) EnsureViewClusterRole(ctx context.Context, serviceAccountName, serviceAccountNamespace string) error {
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	if serviceAccountNamespace == "" {
		serviceAccountNamespace = "default"
	}

	clusterRoleName := fmt.Sprintf("view-pods-%s", strings.ToLower(serviceAccountName))
	bindingName := fmt.Sprintf("%s-view-binding", clusterRoleName)

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		}},
	}

	existingRole, err := e.client.RbacV1().ClusterRoles().Get(ctx, clusterRoleName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = e.client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		existingRole.Rules = clusterRole.Rules
		_, err = e.client.RbacV1().ClusterRoles().Update(ctx, existingRole, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: bindingName},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      serviceAccountName,
			Namespace: serviceAccountNamespace,
		}},
	}

	existingBinding, err := e.client.RbacV1().ClusterRoleBindings().Get(ctx, bindingName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = e.client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		existingBinding.Subjects = binding.Subjects
		existingBinding.RoleRef = binding.RoleRef
		_, err = e.client.RbacV1().ClusterRoleBindings().Update(ctx, existingBinding, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

type errorPodLister struct {
	err error
}

func (e errorPodLister) ListPods(ctx context.Context) ([]podSummary, error) {
	return nil, e.err
}

type errorRBACEnforcer struct {
	err error
}

func (e errorRBACEnforcer) EnsureViewClusterRole(ctx context.Context, serviceAccountName, serviceAccountNamespace string) error {
	return e.err
}

type ensureViewRequest struct {
	ServiceAccountName      string `json:"serviceAccountName,omitempty"`
	ServiceAccountNamespace string `json:"serviceAccountNamespace,omitempty"`
}

type ensureViewResponse struct {
	Message                 string `json:"message"`
	ServiceAccountName      string `json:"serviceAccountName"`
	ServiceAccountNamespace string `json:"serviceAccountNamespace"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	router := buildRouter(nil, nil)
	log.Println("listening on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func buildRouter(podListerInstance podLister, rbacEnforcerInstance rbacEnforcer) *gin.Engine {
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":   "Kubernetes pod listing service",
			"endpoints": []string{"/pods", "/rbac/ensure-view"},
		})
	})
	router.GET("/pods", func(c *gin.Context) {
		handlePods(c, podListerInstance, rbacEnforcerInstance)
	})
	router.POST("/rbac/ensure-view", func(c *gin.Context) {
		handleEnsureView(c, rbacEnforcerInstance)
	})
	return router
}

func handlePods(c *gin.Context, lister podLister, enforcer rbacEnforcer) {
	if lister == nil {
		lister = newPodLister()
	}
	if enforcer == nil {
		enforcer = newRBACEnforcer()
	}

	pods, err := lister.ListPods(c.Request.Context())
	if err != nil && shouldRetryWithRBAC(err) {
		if ensureErr := ensureViewAccessForCurrentServiceAccount(c.Request.Context(), enforcer); ensureErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to grant pod view access: %v", ensureErr)})
			return
		}
		pods, err = lister.ListPods(c.Request.Context())
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list pods: %v", err)})
		return
	}

	c.JSON(http.StatusOK, pods)
}

func shouldRetryWithRBAC(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "forbidden") || strings.Contains(message, "unauthorized") || strings.Contains(message, "cannot access")
}

func handleEnsureView(c *gin.Context, enforcer rbacEnforcer) {
	if enforcer == nil {
		enforcer = newRBACEnforcer()
	}

	var req ensureViewRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serviceAccountName, serviceAccountNamespace := resolveServiceAccount(req)
	if err := enforcer.EnsureViewClusterRole(c.Request.Context(), serviceAccountName, serviceAccountNamespace); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ensureViewResponse{
		Message:                 "ClusterRole and ClusterRoleBinding created",
		ServiceAccountName:      serviceAccountName,
		ServiceAccountNamespace: serviceAccountNamespace,
	})
}

func resolveServiceAccount(req ensureViewRequest) (string, string) {
	serviceAccountName := req.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}

	serviceAccountNamespace := req.ServiceAccountNamespace
	if serviceAccountNamespace == "" {
		serviceAccountNamespace = os.Getenv("POD_NAMESPACE")
	}
	if serviceAccountNamespace == "" {
		serviceAccountNamespace = "default"
	}

	return serviceAccountName, serviceAccountNamespace
}

func newPodLister() podLister {
	client, err := newKubeClient()
	if err != nil {
		return errorPodLister{err: err}
	}
	return kubePodLister{client: client}
}

func ensureViewAccessForCurrentServiceAccount(ctx context.Context, enforcer rbacEnforcer) error {
	serviceAccountName := os.Getenv("SERVICE_ACCOUNT_NAME")
	serviceAccountNamespace := os.Getenv("SERVICE_ACCOUNT_NAMESPACE")
	if serviceAccountNamespace == "" {
		serviceAccountNamespace = os.Getenv("POD_NAMESPACE")
	}
	if serviceAccountNamespace == "" {
		if namespaceBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			serviceAccountNamespace = strings.TrimSpace(string(namespaceBytes))
		}
	}
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	if serviceAccountNamespace == "" {
		serviceAccountNamespace = "default"
	}
	return enforcer.EnsureViewClusterRole(ctx, serviceAccountName, serviceAccountNamespace)
}

func newRBACEnforcer() rbacEnforcer {
	client, err := newKubeClient()
	if err != nil {
		return errorRBACEnforcer{err: err}
	}
	return kubeRBACEnforcer{client: client}
}

func newKubeClient() (kubernetes.Interface, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err := buildConfigFromFlags("", kubeconfig)
		if err == nil {
			return newKubeClientFromConfig(config)
		}
	}

	config, err := buildConfigFromFlags("", "")
	if err == nil {
		return newKubeClientFromConfig(config)
	}

	config, err = inClusterConfig()
	if err != nil {
		return nil, err
	}
	return newKubeClientFromConfig(config)
}
