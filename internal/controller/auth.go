package controller

import (
	"context"
	"fmt"

	operatorv1alpha1 "github.com/k1s-project/k1s-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultControllerReadTokenKey  = "controllerReadToken"
	defaultControllerWriteTokenKey = "controllerWriteToken"
	defaultCABundleKey             = "ca.crt"
)

type apiCredentials struct {
	readToken  string
	writeToken string
	caBundle   []byte
}

func loadCredentials(ctx context.Context, kube client.Client, ownerNamespace string, ref *operatorv1alpha1.K1sAuthSecretRef) (apiCredentials, error) {
	if ref == nil {
		return apiCredentials{}, nil
	}
	namespace := ref.NamespaceOr(ownerNamespace)
	secret := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return apiCredentials{}, err
	}
	readKey := ref.ControllerReadTokenKey
	if readKey == "" {
		readKey = defaultControllerReadTokenKey
	}
	writeKey := ref.ControllerWriteTokenKey
	if writeKey == "" {
		writeKey = defaultControllerWriteTokenKey
	}
	caKey := ref.CABundleKey
	if caKey == "" {
		caKey = defaultCABundleKey
	}
	creds := apiCredentials{}
	if raw, ok := secret.Data[readKey]; ok {
		creds.readToken = string(raw)
	}
	if raw, ok := secret.Data[writeKey]; ok {
		creds.writeToken = string(raw)
	}
	if raw, ok := secret.Data[caKey]; ok {
		creds.caBundle = append([]byte(nil), raw...)
	}
	if ref.ControllerReadTokenKey != "" && creds.readToken == "" {
		return creds, fmt.Errorf("secret %s/%s missing key %q", namespace, ref.Name, readKey)
	}
	return creds, nil
}
