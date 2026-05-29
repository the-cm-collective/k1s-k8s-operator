# Security and RBAC Guide

The operator is designed so Kubernetes RBAC is the tenant-facing policy boundary and k1s tokens stay inside the operator namespace.

## Default Install Scope

The default manifests install namespace-scoped RBAC:

- `ServiceAccount`: `k1s-operator-controller-manager`
- `Role`: operator permissions in `k1s-operator-system`
- `RoleBinding`: binds only the operator ServiceAccount
- no default `ClusterRole`
- no default `ClusterRoleBinding`

The manager watches the namespace from `WATCH_NAMESPACE`. In the default manifest this is populated from the pod namespace.

The controller pod also installs a default ingress-deny NetworkPolicy and binds metrics to `127.0.0.1:8080`. Clusters that scrape controller metrics should add an explicit metrics exposure path and matching NetworkPolicy instead of relying on the default pod network.

## Token Separation

Use separate keys for read and write credentials:

| Secret key | Used for |
| --- | --- |
| `controllerReadToken` | health, status, nodes, app discovery, inference discovery |
| `controllerWriteToken` | `K1sApp`, `K1sInferenceEndpoint`, `K1sResourceSet`, and finalizer cleanup |
| `ca.crt` | optional private CA bundle for HTTPS controller URLs |

Read-only installs should omit `controllerWriteToken` and only grant user RBAC to read-only resources.

## K1sResourceSet Kind Policy

`K1sResourceSet` has two policy layers:

- the operator cap from `--resourceset-allowed-kinds` / `RESOURCESET_ALLOWED_KINDS`;
- the per-resource `spec.allowedKinds` list.

The per-resource list can only narrow the operator cap. A tenant cannot create a `K1sResourceSet` that widens the operator's configured kind set. The public default cap is `Deployment,InferenceCell,InferenceCellSet`.

For production multi-tenant installs, grant `k1sresourcesets` write access only to users that are allowed to manage every kind in the operator cap, or narrow the manager Deployment's `RESOURCESET_ALLOWED_KINDS` value for that installation.

## Kubernetes User RBAC

Grant Kubernetes users only the CRD verbs they need. Example read-only observer:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: k1s-observer
  namespace: k1s-operator-system
rules:
  - apiGroups: ["operator.k1s.io"]
    resources:
      - k1sclusters
      - k1sapps
      - k1sexposures
      - k1sinferenceendpoints
      - k1sresourcesets
    verbs: ["get", "list", "watch"]
```

Example standard service author:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: k1s-standard-service-author
  namespace: k1s-operator-system
rules:
  - apiGroups: ["operator.k1s.io"]
    resources: ["k1sapps", "k1sresourcesets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["operator.k1s.io"]
    resources: ["k1sapps/status", "k1sresourcesets/status"]
    verbs: ["get", "list", "watch"]
```

Example AI/ML author:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: k1s-aiml-author
  namespace: k1s-operator-system
rules:
  - apiGroups: ["operator.k1s.io"]
    resources: ["k1sinferenceendpoints", "k1sresourcesets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["operator.k1s.io"]
    resources: ["k1sinferenceendpoints/status", "k1sresourcesets/status"]
    verbs: ["get", "list", "watch"]
```

## Cross-Namespace Reads

The default install does not have broad cluster read access. If `K1sCluster.spec.proxy.serviceRef.namespace` points to another namespace, add an explicit reader role in that namespace for the specific Services and discovery resources needed by the operator.

The MicroK8s `k1s-dev-a` overlay demonstrates this pattern in `config/overlays/microk8s-dev-a`.

## Status and Logging Rules

Do not place secrets in:

- CR status;
- Events;
- labels;
- annotations;
- generated Services, EndpointSlices, or Ingresses;
- logs.

The operator status should report reachability, condition reasons, remote resource phase, and validation errors only.

## Finalizers

Mutation resources use finalizers so k1s-side resources are deleted before Kubernetes CRs disappear. If k1s is unavailable, finalizers remain and status reports the delete error. This is intentional; remove finalizers manually only after deciding to orphan or manually clean up the k1s-side resource.
