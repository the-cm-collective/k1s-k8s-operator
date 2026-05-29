# Troubleshooting Guide

## Check Operator Health

```sh
kubectl -n k1s-operator-system get deploy,pods
kubectl -n k1s-operator-system logs deploy/k1s-operator-controller-manager --tail=200
kubectl -n k1s-operator-system get k1scluster,k1sapp,k1sexposure,k1sinferenceendpoint,k1sresourceset
```

Conditions are the first diagnostic surface. Inspect them before reading controller logs:

```sh
kubectl -n k1s-operator-system describe k1scluster/edge-a
kubectl -n k1s-operator-system describe k1sresourceset/example-bundle
```

## Dry-Run Conflicts

If `kubectl apply --server-side --dry-run=server` reports field ownership conflicts in a live development cluster, rerun with:

```sh
kubectl apply --server-side --dry-run=server --force-conflicts -f /tmp/k1s-operator-default.yaml
```

This verifies schema validity without changing live resources.

## Missing Write Token

Symptoms:

- `CredentialsValid=False`
- message mentions missing write token

Fix:

```sh
kubectl -n k1s-operator-system get secret k1s-edge-a-operator -o jsonpath='{.data.controllerWriteToken}' >/dev/null
```

Then recreate the Secret with `controllerWriteToken` if mutation CRDs are expected to work.

## InferenceCell Rejected As Deployment-Only

Symptom:

```text
Input should be 'Deployment'
spec.image Field required
```

Cause: the running k1s controller image does not support inference manifests on `/apply`.

Fix: roll the target k1s controller to a build that includes the inference API routes, then requeue or update the CR.

## InferenceCell localPath Validation

Symptom:

```text
spec.model.localPath Field required
```

Cause: a raw `InferenceCell` manifest in `K1sResourceSet.spec.manifests` omitted `spec.model.localPath`.

Fix:

```yaml
model:
  modelId: meta-llama/Llama-3.2-1B-Instruct
  localPath: ""
```

`K1sInferenceEndpoint` sets this field for generated manifests.

## not_leader Errors

Symptoms:

- status contains `k1s API POST /apply returned 409`
- message contains `error: not_leader`
- advertised leader host does not resolve

The operator retries advertised leader URLs and then falls back to the configured Service URL with one-shot HTTP connections. If errors persist:

1. Confirm k1s controller pods are healthy.
2. Confirm a current k1s leader exists.
3. Confirm the configured `K1sCluster.spec.controller.url` points at the controller Service.
4. Check whether the k1s leader advertises a DNS name that the operator pod can resolve.

Commands:

```sh
kubectl -n k1s-system get pods -l app.kubernetes.io/component=controller -o wide
kubectl -n k1s-system logs deploy/k1s-controller -c controller --tail=120
kubectl -n k1s-operator-system logs deploy/k1s-operator-controller-manager --tail=120
```

## Exposure Not Ready

Check the proxy Service and EndpointSlices:

```sh
kubectl -n k1s-system get svc k1s-edge-proxy
kubectl -n k1s-system get endpointslice -l kubernetes.io/service-name=k1s-edge-proxy -o wide
kubectl -n k1s-operator-system describe k1sexposure/example
```

If ingress is disabled, `K1sExposure` can still be Ready when Service and EndpointSlice are reconciled.
