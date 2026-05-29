# Installation Guide

This guide installs the operator with namespace-scoped RBAC. Use this path for public repo defaults and most development clusters.

## Prerequisites

- Kubernetes 1.27 or newer.
- `kubectl`, `kustomize`, and the Go toolchain used by `make verify`.
- A reachable k1s controller API URL.
- k1s read token for discovery-only resources.
- k1s write token only when enabling `K1sApp`, `K1sInferenceEndpoint`, or `K1sResourceSet`.
- Optional CA bundle when the k1s controller URL uses a private HTTPS CA.

For AI/ML resources, the k1s controller must support `InferenceCell` and `InferenceCellSet` manifests on `/apply` and expose `/inference/cells/*` and `/inference/cellsets/*` status routes.

## Verify Manifests

```sh
make verify
make kustomize-build
kubectl apply --server-side --dry-run=server -f /tmp/k1s-operator-default.yaml
```

If a live development cluster already has fields owned by another server-side apply manager, dry-run with:

```sh
kubectl apply --server-side --dry-run=server --force-conflicts -f /tmp/k1s-operator-default.yaml
```

## Install

```sh
kubectl apply --server-side -f /tmp/k1s-operator-default.yaml
kubectl -n k1s-operator-system rollout status deployment/k1s-operator-controller-manager --timeout=120s
```

The default install watches `k1s-operator-system` through the downward-api `WATCH_NAMESPACE` environment variable and uses namespace-scoped `Role` and `RoleBinding` resources.

## Credentials Secret

Create credentials in the namespace watched by the operator. Do not reuse the broad k1s bootstrap Secret unless this is a private lab and you understand the blast radius.

```sh
kubectl -n k1s-operator-system create secret generic k1s-dev-a-operator \
  --from-literal=controllerReadToken='<read-token>' \
  --from-literal=controllerWriteToken='<write-token>'
```

Optional CA bundle:

```sh
kubectl -n k1s-operator-system create secret generic k1s-dev-a-operator \
  --from-literal=controllerReadToken='<read-token>' \
  --from-literal=controllerWriteToken='<write-token>' \
  --from-file=ca.crt=./k1s-ca.crt
```

## Minimal K1sCluster

```yaml
apiVersion: operator.k1s.io/v1alpha1
kind: K1sCluster
metadata:
  name: dev-a
  namespace: k1s-operator-system
spec:
  controller:
    url: http://k1s-dev-a-k1s-core-ha-controller.k1s-dev-a.svc:9108
  authSecretRef:
    name: k1s-dev-a-operator
    controllerReadTokenKey: controllerReadToken
    controllerWriteTokenKey: controllerWriteToken
  proxy:
    mode: Service
    serviceRef:
      name: k1s-dev-a-k1s-core-ha-edge-proxy
      namespace: k1s-dev-a
      port: 10080
  pollIntervalSeconds: 15
```

Wait for readiness:

```sh
kubectl -n k1s-operator-system wait --for=condition=Ready k1scluster/dev-a --timeout=120s
```

## Upgrade

Render and apply the new manifests:

```sh
make verify
kustomize build config/default >/tmp/k1s-operator-default.yaml
kubectl apply --server-side --force-conflicts -f /tmp/k1s-operator-default.yaml
kubectl -n k1s-operator-system rollout status deployment/k1s-operator-controller-manager --timeout=120s
```

CRDs are served as `v1alpha1`; check release notes before changing CRD schemas in a shared cluster.

## Uninstall

Delete custom resources first so finalizers can clean up k1s-side resources:

```sh
kubectl -n k1s-operator-system delete k1sresourcesets,k1sinferenceendpoints,k1sexposures,k1sapps --all
kubectl -n k1s-operator-system delete k1sclusters --all
kubectl delete -f /tmp/k1s-operator-default.yaml
```

If finalizers are blocked, inspect the operator logs and k1s controller reachability before removing finalizers manually.
