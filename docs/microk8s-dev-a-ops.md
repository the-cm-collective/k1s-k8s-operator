# MicroK8s dev-a Operations Guide

This guide covers the local validation path that uses MicroK8s plus the sibling `../k1s` `k1s-dev-a` HA stack.

## Namespaces

Monitor these namespaces during live tests:

- `k1s-operator-system`
- `k1s-dev-a`

Useful k9s command:

```sh
k9s -A -n k1s-operator-system,k1s-dev-a
```

## Overlay

The MicroK8s overlay is in `config/overlays/microk8s-dev-a`. It installs the operator into `k1s-operator-system` and grants explicit read access to selected Services and discovery resources in `k1s-dev-a`.

```sh
kustomize build config/overlays/microk8s-dev-a >/tmp/k1s-operator-microk8s-dev-a.yaml
kubectl apply --server-side --force-conflicts -f /tmp/k1s-operator-microk8s-dev-a.yaml
```

## k1s Runtime Requirement

Standard `Deployment` resources work against the older k1s controller API. AI/ML validation requires a k1s controller image that includes:

- `InferenceCell` and `InferenceCellSet` handling in `/apply`;
- `/inference/cells/<namespace>/<name>` status;
- `/inference/cellsets/<namespace>/<name>` status;
- `/inference/delete/*` delete routes.

When the running dev-a image does not include those routes, build and import a source image from `../k1s`, then roll only the controller container:

```sh
podman build \
  -f ../k1s/ops/images/controller.Dockerfile \
  -t reg.microk8s.core.home.arpa:32000/k1s/k1s-core:dev-operator-inference \
  ../k1s

podman save reg.microk8s.core.home.arpa:32000/k1s/k1s-core:dev-operator-inference \
  | microk8s ctr image import -

kubectl -n k1s-dev-a set image \
  deployment/k1s-dev-a-k1s-core-ha-controller \
  controller=reg.microk8s.core.home.arpa:32000/k1s/k1s-core:dev-operator-inference

kubectl -n k1s-dev-a rollout status \
  deployment/k1s-dev-a-k1s-core-ha-controller --timeout=240s
```

The release-style controller Dockerfile is used because the HA Helm deployment expects controller runtime paths to be writable by the process.

## Operator Image Rollout

```sh
docker build -t k1s-operator:live-dev -f Dockerfile .
docker save k1s-operator:live-dev | microk8s ctr image import -
kubectl -n k1s-operator-system set image \
  deployment/k1s-operator-controller-manager \
  manager=k1s-operator:live-dev
kubectl -n k1s-operator-system rollout status \
  deployment/k1s-operator-controller-manager --timeout=120s
```

## Live Validation Gates

Keep the resources up for inspection when running live tests:

```sh
kubectl -n k1s-operator-system wait --for=condition=Ready k1scluster/dev-a-live --timeout=120s
kubectl -n k1s-operator-system wait --for=condition=Ready k1sapp/live-standard --timeout=120s
kubectl -n k1s-operator-system wait --for=condition=Ready k1sexposure/live-standard --timeout=120s
kubectl -n k1s-operator-system wait --for=condition=Ready k1sinferenceendpoint/live-infer --timeout=120s
kubectl -n k1s-operator-system wait --for=condition=Ready k1sresourceset/live-standard-bundle --timeout=120s
kubectl -n k1s-operator-system wait --for=condition=Ready k1sresourceset/live-bundle --timeout=120s
```

Inspect status:

```sh
kubectl -n k1s-operator-system get \
  k1scluster,k1sapp,k1sexposure,k1sinferenceendpoint,k1sresourceset

kubectl -n k1s-operator-system describe k1sresourceset/live-bundle
```

The mixed ResourceSet AI/ML manifest must include `spec.model.localPath`, even if it is an empty string, because current k1s validation requires the field.

## Leave-Up Policy

For operator integration review, leave these namespaces and CRs running until inspection is complete:

- `k1s-operator-system`
- `k1s-dev-a`
- `K1sCluster/dev-a-live`
- `K1sApp/live-standard`
- `K1sExposure/live-standard`
- `K1sInferenceEndpoint/live-infer`
- `K1sResourceSet/live-standard-bundle`
- `K1sResourceSet/live-bundle`
