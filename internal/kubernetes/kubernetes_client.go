package kubernetes

import (
	"errors"
	"fmt"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	k8sClient "k8s.io/client-go/kubernetes"
)

func NewKubernetesRestConfig() (*rest.Config, error) {
	config, inClusterErr := rest.InClusterConfig()
	if inClusterErr == nil {
		return config, nil
	}

	// fall-back to kubeconfig if not running in cluster
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, kubeConfigErr := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if kubeConfigErr != nil {
		return nil, fmt.Errorf("failed to create kube client config: %w", errors.Join(inClusterErr, kubeConfigErr))
	}

	return config, nil
}

func NewKubernetesClient() (k8sClient.Interface, error) {
	config, err := NewKubernetesRestConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := k8sClient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
