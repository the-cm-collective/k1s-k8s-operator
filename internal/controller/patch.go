package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const lastAppliedHashAnnotation = "operator.k1s.io/last-applied-hash"

func patchStatus(ctx context.Context, kube client.Client, obj client.Object, mutate func(client.Object)) error {
	key := client.ObjectKeyFromObject(obj)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := obj.DeepCopyObject().(client.Object)
		if err := kube.Get(ctx, key, current); err != nil {
			return err
		}
		before := current.DeepCopyObject().(client.Object)
		mutate(current)
		err := kube.Status().Patch(ctx, current, client.MergeFrom(before))
		if errors.IsConflict(err) {
			return err
		}
		return err
	})
}

func patchObject(ctx context.Context, kube client.Client, obj client.Object, mutate func(client.Object)) error {
	key := client.ObjectKeyFromObject(obj)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := obj.DeepCopyObject().(client.Object)
		if err := kube.Get(ctx, key, current); err != nil {
			return err
		}
		before := current.DeepCopyObject().(client.Object)
		mutate(current)
		err := kube.Patch(ctx, current, client.MergeFrom(before))
		if errors.IsConflict(err) {
			return err
		}
		return err
	})
}

func patchFinalizers(ctx context.Context, kube client.Client, obj client.Object, finalizers []string) error {
	return patchObject(ctx, kube, obj, func(current client.Object) {
		current.SetFinalizers(finalizers)
	})
}

func patchLastAppliedHash(ctx context.Context, kube client.Client, obj client.Object, hash string) error {
	return patchObject(ctx, kube, obj, func(current client.Object) {
		annotations := current.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[lastAppliedHashAnnotation] = hash
		current.SetAnnotations(annotations)
	})
}

func lastAppliedHash(obj client.Object) string {
	if obj.GetAnnotations() == nil {
		return ""
	}
	return obj.GetAnnotations()[lastAppliedHashAnnotation]
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func hashManifestSet(manifests [][]byte) string {
	digest := sha256.New()
	for _, manifest := range manifests {
		digest.Write([]byte{0})
		digest.Write(manifest)
	}
	return hex.EncodeToString(digest.Sum(nil))
}
