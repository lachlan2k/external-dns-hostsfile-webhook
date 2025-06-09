package main

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

const configMapHostsfileName = "hosts"

type ConfigMapHostsfilePersister struct {
	namespace       string
	name            string
	configMapClient v1.ConfigMapInterface
}

func NewConfigMapHostsfilePersister(namespace, name string) (*ConfigMapHostsfilePersister, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error creating in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	configMapClient := clientset.CoreV1().ConfigMaps(namespace)

	return &ConfigMapHostsfilePersister{
		namespace:       namespace,
		name:            name,
		configMapClient: configMapClient,
	}, nil
}

func (p *ConfigMapHostsfilePersister) Read() (string, error) {
	cm, err := p.configMapClient.Get(context.Background(), p.name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error reading ConfigMap %s/%s: %v", p.namespace, p.name, err)
	}

	contents, ok := cm.Data[configMapHostsfileName]
	if !ok {
		return "", fmt.Errorf("ConfigMap %s/%s does not contain key %s", p.namespace, p.name, configMapHostsfileName)
	}

	return contents, nil
}

func (p *ConfigMapHostsfilePersister) Write(contents string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: p.name,
		},
		Data: map[string]string{
			configMapHostsfileName: contents,
		},
	}

	_, err := p.configMapClient.Update(context.Background(), cm, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("Warn: failed to update ConfigMap %s/%s: %v\n", p.namespace, p.name, err)
		log.Println("Attempting to create ConfigMap instead...")

		_, err = p.configMapClient.Create(context.Background(), cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("couldn't update or create ConfigMap %s/%s: %v", p.namespace, p.name, err)
		}
		log.Printf("Created ConfigMap %s/%s successfully\n", p.namespace, p.name)
	}

	return nil
}
