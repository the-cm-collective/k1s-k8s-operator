# k1s Kubernetes Operator Plan

Status: implementation draft
Date: 2026-05-28
Repository: `k1s-operator`
Sibling reference implementation: `../k1s`

## Purpose

The operator should position k1s as a Kubernetes-adjacent runtime and edge fabric that can be consumed from an existing Kubernetes cluster. The goal is not to make k1s replace Kubernetes in places where Kubernetes is already the right control plane. The goal is to let Kubernetes users safely discover selected k1s workloads, expose them through Kubernetes-native resources, and observe their state through Kubernetes-native status.

The first version should be intentionally narrow:

- read-only toward k1s;
- opt-in per exposed k1s app;
- HTTP/S exposure only;
- proxy-backed traffic, not direct pod or replica endpoint assumptions;
- Kubernetes-native resources generated in the namespace where the exposure object lives.

This keeps the operator useful in the current microk8s lab while leaving room for API shim, Gateway API, L4, and richer management features later.

The second checkpoint set should extend the operator from read-only exposure into RBAC-scoped capability delegation. The goal is not to fully integrate k1s with the Kubernetes scheduler. Kubernetes workloads and tenants should be able to create, discover, update, and delete selected k1s-side resources through Kubernetes CRDs and normal Kubernetes RBAC, while the operator remains the only component that holds scoped k1s write credentials. This is useful for both standard services delivered through k1s and AI/ML workflows such as inference APIs, model serving endpoints, and GPU-backed runtime cells.

## Local Lab Baseline

Current development cluster observations:

- Kubernetes context: `microk8s`.
- k1s HA lab namespace: `k1s-dev-a`.
- k1s controller deployment: `k1s-dev-a-k1s-core-ha-controller`, 3 available replicas.
- k1s API shim deployment: `k1s-dev-a-k1s-core-ha-apishim`, 1 available replica.
- Controller Service: `k1s-dev-a-k1s-core-ha-controller`, ports `9108` and `9110`.
- API shim Service: `k1s-dev-a-k1s-core-ha-apishim`, port `8445`.
- Edge proxy Service: `k1s-dev-a-k1s-core-ha-edge-proxy`, port `10080`.
- Edge proxy EndpointSlice currently has ready IPv4 pod endpoints on port `10080`.
- Bootstrap ConfigMap: `k1s-dev-a-k1s-core-ha-bootstrap`.
- Bootstrap values include:
  - `api_url: https://api.k1s-dev-a.core.home.arpa/`
  - `stack_domain: k1s-dev-a.core.home.arpa`
  - `wildcard_apps_domain: *.apps.k1s-dev-a.core.home.arpa`
  - `auth_secret_name: k1s-dev-a-k1s-core-ha-auth`
  - `controller_external_service: k1s-dev-a-k1s-core-ha-controller-external`
  - `rathole_external_service: k1s-dev-a-k1s-core-ha-rathole`
- Kubernetes resources available in the local cluster include:
  - `discovery.k8s.io/v1` `EndpointSlice`
  - `networking.k8s.io/v1` `Ingress`
- Gateway API resources are not installed in the local microk8s cluster at the time of this plan.

The operator should be developed in a separate namespace, for example `k1s-operator-dev`, and should reference the existing k1s lab stack rather than modifying it.

## k1s Interfaces To Use

The sibling k1s project already exposes the surfaces needed for a read-only bridge:

- Controller-native HTTP API:
  - `GET /health`
  - `GET /status`
  - `GET /status/<app>`
  - `GET /manifest/<app>`
  - `GET /nodes`
  - `GET /system`
  - `GET /metrics`
- Controller auth:
  - read routes are open by default, but require bearer auth when controller tokens are configured;
  - `AE_API_READ_TOKEN` is the correct minimum role for operator reads;
  - mutation routes require separate mutation enablement and stronger roles and are out of scope for v1.
- API shim:
  - provides Kubernetes-compatible discovery, Services, Ingress, Endpoints, EndpointSlice, Pods, Deployments, and watch-like behavior;
  - supports read/list/watch tokens via `AE_APISHIM_READ_TOKEN`;
  - should remain optional for v1, because the controller HTTP API is enough for app and cluster status.
- Ingress and proxy topology:
  - k1s supports core proxy, core-to-edge-public, edge-local, and core-local ingress modes;
  - `core-proxy` is the default NAT-friendly path where core Envoy proxies to edge workloads through Rathole;
  - direct edge or pod endpoint reachability should not be assumed from a Kubernetes cluster.
- L4 services:
  - k1s explicitly treats multi-replica TCP/UDP exposure as external-proxy scope;
  - v1 should not try to create a general L4 bridge.

Reference docs in `../k1s`:

- `docs/reference/http-api.md`
- `docs/reference/api-auth.md`
- `docs/reference/ingress.md`
- `docs/reference/apishim-compatibility-matrix.md`
- `docs/adr/0003-l4-services-scope.md`
- `docs/adr/0016-core-edge-overlay-ingress-modes.md`

## Requirements

### Functional Requirements

1. Represent a reachable k1s control plane from Kubernetes.
2. Represent selected k1s apps as Kubernetes-side exposure objects.
3. Create Kubernetes-native routing resources for selected apps:
   - selectorless `Service`;
   - managed `EndpointSlice`;
   - optional `Ingress`.
4. Route generated Services to a k1s proxy endpoint, not directly to k1s pods.
5. Mirror app readiness and basic app metadata into Kubernetes status.
6. Mirror cluster reachability, HA health, node summary, and proxy readiness into Kubernetes status.
7. Support same-namespace and explicitly configured cross-namespace references where RBAC allows them.
8. Emit Kubernetes Events for meaningful state transitions and failure cases.
9. Reconcile continuously and recover from deleted generated resources.
10. Clean up generated resources when the owning custom resource is deleted.
11. In Stage 2, allow authorized Kubernetes users and workloads to CRUD selected k1s resources through operator-owned CRDs.
12. In Stage 2, support both standard service/workload resources and AI/ML inference resources.
13. In Stage 2, surface k1s-side discovery and status through Kubernetes status without allowing the Kubernetes scheduler to place workloads on k1s nodes.

### Security Requirements

1. The operator must use least-privilege k1s credentials.
2. The recommended Secret must contain only read credentials, not the broad HA bootstrap Secret.
3. Stage 1 must not call k1s mutation endpoints.
4. Stage 2 mutation must be explicitly enabled by scoped k1s write credentials and Kubernetes RBAC on the mutating CRDs.
5. v1 must not require API shim admin credentials.
6. TLS verification must be enabled by default when using HTTPS URLs.
7. CA bundles must be configurable through a Secret or ConfigMap reference.
8. Generated Kubernetes resources must be owned by their custom resource where Kubernetes owner-reference rules permit it.
9. Cross-namespace reads must be explicit and covered by operator RBAC.
10. Tokens must never be written into status, Events, logs, labels, annotations, or generated resources.
11. Stage 2 must separate read tokens and write tokens so read-only exposure installs do not accidentally receive mutation authority.

### Operational Requirements

1. The operator should run as a standard controller-runtime based Kubernetes controller.
2. CRDs and RBAC should be installable with plain Kubernetes YAML and kustomize.
3. The default development path should work on local microk8s.
4. The implementation must be testable without modifying the existing k1s HA lab.
5. The lab path should use the existing `k1s-dev-a` edge proxy Service as the v1 traffic target.
6. The operator must surface degraded states clearly through conditions.
7. Requeue intervals must be configurable and conservative by default.
8. Generated object names must be deterministic, stable, and overridable where needed.

## Non-Goals For v1

- Replacing k1s desired-state management with Kubernetes CRDs.
- Applying k1s app manifests from Kubernetes in the Stage 1 read-only bridge.
- Scaling, deleting, pausing, resuming, or execing into k1s workloads.
- General TCP/UDP or LoadBalancer bridging.
- Direct mirroring of k1s pod or replica IPs as the primary traffic path.
- Assuming k1s overlay, WireGuard, Rathole, or pod networks are directly reachable from the Kubernetes cluster.
- Installing Gateway API.
- Owning DNS records.
- Creating or modifying k1s ingress routes.
- Full API shim synchronization.
- Kubernetes conformance.
- Presenting k1s nodes as Kubernetes schedulable nodes.
- Letting the Kubernetes controller manager or scheduler directly place workloads on k1s.

## API Design

All custom resources should use:

```yaml
apiVersion: operator.k1s.io/v1alpha1
```

All resources are namespaced. Namespaced resources keep tenancy, RBAC, cleanup, and ownership understandable for v1.

### K1sCluster

`K1sCluster` describes a k1s control plane and the proxy endpoint used to reach exposed k1s workloads.

Example:

```yaml
apiVersion: operator.k1s.io/v1alpha1
kind: K1sCluster
metadata:
  name: dev-a
  namespace: k1s-operator-dev
spec:
  controller:
    url: https://api.k1s-dev-a.core.home.arpa/
  apishim:
    url: https://k1s-dev-a-k1s-core-ha-apishim.k1s-dev-a.svc:8445
  bootstrapConfigMapRef:
    name: k1s-dev-a-k1s-core-ha-bootstrap
    namespace: k1s-dev-a
  authSecretRef:
    name: k1s-dev-a-operator-read
    namespace: k1s-operator-dev
    controllerReadTokenKey: controllerReadToken
    apishimReadTokenKey: apishimReadToken
    caBundleKey: ca.crt
  proxy:
    mode: Service
    serviceRef:
      name: k1s-dev-a-k1s-core-ha-edge-proxy
      namespace: k1s-dev-a
      port: 10080
  pollIntervalSeconds: 15
```

Spec fields:

| Field | Required | Description |
| --- | --- | --- |
| `controller.url` | yes | Base URL for the k1s controller HTTP API. |
| `apishim.url` | no | Optional API shim URL for future read/list/watch integrations. |
| `bootstrapConfigMapRef` | no | Optional reference to a k1s Helm bootstrap ConfigMap. Used for defaults and status context. |
| `authSecretRef` | no | Reference to read-only credentials and optional CA material. |
| `proxy.mode` | yes | `Service` or `External`. |
| `proxy.serviceRef` | when mode is `Service` | Kubernetes Service whose ready EndpointSlices become the generated exposure backends. |
| `proxy.external` | when mode is `External` | Explicit addresses or DNS name for an externally reachable k1s proxy. |
| `pollIntervalSeconds` | no | Reconcile interval. Defaults to a conservative value such as 30 seconds. |

Status fields:

| Field | Description |
| --- | --- |
| `observedGeneration` | Last reconciled generation. |
| `controllerAvailable` | Whether the controller health/status surface is reachable. |
| `apishimAvailable` | Whether the optional API shim is reachable. |
| `proxyReady` | Whether the configured proxy has at least one usable backend. |
| `stackDomain` | Domain discovered from bootstrap config, when available. |
| `wildcardAppsDomain` | Wildcard app domain discovered from bootstrap config, when available. |
| `nodeSummary` | Ready/stale/total node summary from `/nodes` or `/system`. |
| `conditions` | Standard Kubernetes conditions. |

Required conditions:

- `Ready`
- `ControllerAvailable`
- `ProxyReady`
- `CredentialsValid`
- `BootstrapLoaded`
- `ApishimAvailable`

### K1sExposure

`K1sExposure` opts a single k1s app into Kubernetes-side exposure.

Example:

```yaml
apiVersion: operator.k1s.io/v1alpha1
kind: K1sExposure
metadata:
  name: echo
  namespace: k1s-operator-dev
spec:
  clusterRef:
    name: dev-a
  appRef:
    namespace: default
    name: echo
  host: echo.apps.k1s-dev-a.core.home.arpa
  path: /
  service:
    name: echo-k1s
    port: 80
  ingress:
    enabled: true
    className: nginx
    tlsSecretName: echo-tls
    annotations: {}
```

Spec fields:

| Field | Required | Description |
| --- | --- | --- |
| `clusterRef.name` | yes | Referenced `K1sCluster`. |
| `clusterRef.namespace` | no | Optional namespace. Defaults to the exposure namespace. |
| `appRef.name` | yes | k1s app name. |
| `appRef.namespace` | no | k1s app namespace. Defaults to `default`. |
| `host` | yes | Kubernetes Ingress host and expected k1s route host. |
| `path` | no | Ingress path. Defaults to `/`. |
| `service.name` | no | Generated Service name. Defaults from exposure name. |
| `service.port` | no | Generated Service port. Defaults to `80`. |
| `ingress.enabled` | no | Whether to generate an Ingress. Defaults to `true`. |
| `ingress.className` | no | Optional Ingress class. |
| `ingress.tlsSecretName` | no | Optional Kubernetes TLS Secret for Ingress termination. |
| `ingress.annotations` | no | Extra annotations copied to the generated Ingress. |

Status fields:

| Field | Description |
| --- | --- |
| `observedGeneration` | Last reconciled generation. |
| `appReady` | Whether the referenced app is ready according to k1s. |
| `serviceName` | Generated Service name. |
| `endpointSliceName` | Generated EndpointSlice name. |
| `ingressName` | Generated Ingress name, when enabled. |
| `proxyEndpointCount` | Number of ready proxy endpoints written. |
| `lastAppSyncTime` | Last successful app status read. |
| `conditions` | Standard Kubernetes conditions. |

Required conditions:

- `Ready`
- `ClusterReady`
- `AppFound`
- `AppReady`
- `ProxyEndpointsReady`
- `ResourcesApplied`
- `IngressReady`

### K1sAppMirror

`K1sAppMirror` is an operator-owned record of the k1s app state observed for one exposure. It should be created and maintained by the operator rather than by users.

Example:

```yaml
apiVersion: operator.k1s.io/v1alpha1
kind: K1sAppMirror
metadata:
  name: echo
  namespace: k1s-operator-dev
  ownerReferences:
    - apiVersion: operator.k1s.io/v1alpha1
      kind: K1sExposure
      name: echo
spec:
  clusterRef:
    name: dev-a
  appRef:
    namespace: default
    name: echo
  exposureRef:
    name: echo
status:
  ready: true
  replicas:
    desired: 2
    ready: 2
  image: ghcr.io/example/echo:latest
  revision: "7"
  observedHost: echo.apps.k1s-dev-a.core.home.arpa
  placements:
    - node: edge-1
      ready: true
```

The mirror exists so users can inspect app state without overloading the exposure status. It also gives later versions a natural place to add richer app inventory, placement, rollout, and policy status.

### Stage 2 Capability Resources

Stage 2 adds mutating, RBAC-scoped resources. These resources are still Kubernetes-side delegation objects, not Kubernetes scheduling primitives. The Kubernetes API server stores desired intent, the operator translates allowed intent into k1s controller requests, and k1s remains responsible for placement, execution, networking, and runtime-specific lifecycle.

`K1sApp` is the standard workload path. It accepts an allowlisted k1s app/deployment manifest, applies it through the k1s controller with a write token, reports accepted/applied/ready conditions, and optionally pairs with a `K1sExposure` for Kubernetes-side traffic. This covers ordinary HTTP services and other standard services that k1s can deliver.

`K1sInferenceEndpoint` is the AI/ML path. It describes model identity, tensor/pipeline parallelism, executor preferences, fabric policy, runtime class hints, and optional cell set replication. The operator renders this into k1s-native inference resources such as `InferenceCell` or `InferenceCellSet`. Status should report the active executor, serving endpoint, cell/cell-set name, readiness, and the last k1s-side error.

`K1sResourceSet` is the advanced escape hatch. It accepts multiple native k1s manifests but only applies kinds allowed by policy. Its default allowlist should cover standard deployments and inference resources. This keeps the operator extensible while still avoiding a broad arbitrary-write surface.

The most viable integration path between the Kubernetes-side AE and the k1s-side AE is:

1. Keep the Kubernetes-facing contract as CRDs plus ordinary Kubernetes RBAC.
2. Keep k1s mutation authority inside the operator through scoped write tokens stored in Secrets.
3. Use k1s controller APIs for standard `Deployment` apply/delete first, because the current controller already has the closest matching surface.
4. Add or stabilize a k1s-side mutation API for inference resources before treating `K1sInferenceEndpoint` as production-ready.
5. Use status and discovery as the contract between the AEs: Kubernetes users see CRD conditions and mirrors; k1s owns placement, execution, and runtime detail.

## Generated Kubernetes Resources

For each `K1sExposure`, the operator should generate resources in the exposure namespace.

### Service

The generated Service should be selectorless. It represents a stable Kubernetes service name and port for a selected k1s app, but traffic is sent to the k1s proxy endpoint.

Required Service behavior:

- type `ClusterIP`;
- no selector;
- deterministic name;
- one HTTP port for v1;
- port defaults to `80`;
- target port matches the generated EndpointSlice port;
- owner reference to the `K1sExposure`.

### EndpointSlice

The generated EndpointSlice should point at ready k1s proxy endpoints.

Required EndpointSlice behavior:

- label `kubernetes.io/service-name` must match the generated Service;
- label ownership must identify the operator and exposure;
- endpoints come from `K1sCluster.spec.proxy`;
- `Service` mode copies ready endpoints from the configured proxy Service EndpointSlices;
- `External` mode uses configured IP addresses or resolves configured DNS names;
- endpoint readiness must reflect proxy endpoint readiness, not app readiness;
- no k1s app token or control-plane data appears in labels or annotations.

For the current lab, v1 should use:

```yaml
proxy:
  mode: Service
  serviceRef:
    namespace: k1s-dev-a
    name: k1s-dev-a-k1s-core-ha-edge-proxy
    port: 10080
```

### Ingress

The generated Ingress should route Kubernetes ingress traffic to the generated Service.

Required Ingress behavior:

- use `networking.k8s.io/v1`;
- one host and one path in v1;
- `pathType: Prefix`;
- optional `ingressClassName`;
- optional TLS Secret reference;
- deterministic name;
- owner reference to the `K1sExposure`.

Important limitation: v1 assumes the selected host/path is already meaningful to the k1s proxy and k1s ingress layer. The operator creates the Kubernetes-side route to the k1s proxy; it does not create or mutate the k1s-side app ingress route.

## Traffic Model

The v1 traffic model is:

```text
client
  -> Kubernetes Ingress
  -> generated selectorless Service
  -> generated EndpointSlice
  -> k1s core/edge proxy endpoint
  -> k1s ingress/proxy fabric
  -> selected k1s app
```

This model intentionally avoids direct use of k1s pod, container, or edge node endpoints. It works with NAT-friendly core-proxy deployments and gives Kubernetes one stable endpoint set to route to.

## Reconciler Design

### K1sCluster Reconciler

Inputs:

- `K1sCluster`;
- optional auth Secret;
- optional bootstrap ConfigMap;
- proxy Service and EndpointSlices when `proxy.mode` is `Service`.

Reconcile steps:

1. Load and validate referenced Secret keys.
2. Load bootstrap ConfigMap if configured.
3. Build a controller HTTP client with bearer token and CA settings.
4. Call controller health and status endpoints.
5. Call `/nodes` and `/system` when available.
6. Validate proxy readiness:
   - for `Service` mode, read proxy Service EndpointSlices and count ready endpoints;
   - for `External` mode, validate configured addresses or resolve configured DNS name.
7. Set status fields and conditions.
8. Requeue after `pollIntervalSeconds`.

Failure behavior:

- invalid credentials: `CredentialsValid=False`, `Ready=False`;
- controller unavailable: `ControllerAvailable=False`, `Ready=False`;
- proxy has no usable endpoints: `ProxyReady=False`, `Ready=False`;
- bootstrap missing: `BootstrapLoaded=False`, but only fatal if required fields are missing from explicit spec.

### K1sExposure Reconciler

Inputs:

- `K1sExposure`;
- referenced `K1sCluster`;
- k1s controller app status;
- proxy endpoints from cluster proxy configuration.

Reconcile steps:

1. Resolve `clusterRef`.
2. Verify referenced cluster is ready enough to expose traffic.
3. Read app status from k1s using `GET /status/<app>`.
4. Optionally read app manifest using `GET /manifest/<app>`.
5. Build or update `K1sAppMirror`.
6. Build or update generated Service.
7. Build or update generated EndpointSlice.
8. Build or update generated Ingress if enabled.
9. Set exposure status and conditions.
10. Emit Events for key state transitions.

Failure behavior:

- missing cluster: `ClusterReady=False`, `Ready=False`;
- app not found: `AppFound=False`, no generated traffic endpoints should be considered ready;
- app not ready: `AppReady=False`, generated resources can exist but `Ready=False`;
- proxy unavailable: `ProxyEndpointsReady=False`, generated Service should have no ready endpoints;
- Ingress apply failure: `ResourcesApplied=False` or `IngressReady=False`.

### K1sAppMirror Reconciler

`K1sAppMirror` can be maintained by the `K1sExposure` reconciler in v1. A separate reconciler is not required unless mirrors become user-created or watch-driven later.

## RBAC

The operator needs permission to:

- read its CRDs;
- update CRD status subresources;
- create/update/delete generated Services, EndpointSlices, Ingresses, and Events;
- read referenced Secrets and ConfigMaps;
- read proxy Services and EndpointSlices, potentially in the k1s namespace.

The default RBAC should be namespace-scoped where possible. Cross-namespace lab access can be granted explicitly for:

- `k1s-dev-a` ConfigMap reads;
- `k1s-dev-a` Service and EndpointSlice reads;
- no default access to k1s broad auth Secrets.

Recommended credential model:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: k1s-dev-a-operator-read
  namespace: k1s-operator-dev
type: Opaque
stringData:
  controllerReadToken: replace-me
  apishimReadToken: replace-me-if-used
```

## Labels And Ownership

Use stable labels on generated objects:

```yaml
app.kubernetes.io/name: k1s-operator
app.kubernetes.io/managed-by: k1s-operator
operator.k1s.io/cluster: <cluster-name>
operator.k1s.io/exposure: <exposure-name>
operator.k1s.io/app-namespace: <k1s-app-namespace>
operator.k1s.io/app-name: <k1s-app-name>
```

EndpointSlices must also include:

```yaml
kubernetes.io/service-name: <generated-service-name>
```

Avoid labels or annotations that contain URLs with credentials, bearer tokens, raw manifests, or large app status payloads.

## Implementation Plan

### Phase 0: Planning Artifact

Deliverables:

- `PLAN.md` with requirements, technical design, and implementation plan.

Validation:

- file exists;
- content reflects local lab and sibling k1s architecture;
- no secrets embedded.

### Phase 1: Operator Scaffold And API Types

Deliverables:

- Go module;
- controller-runtime manager;
- CRD Go types for `K1sCluster`, `K1sExposure`, and `K1sAppMirror`;
- generated CRD YAML;
- RBAC manifests;
- sample resources for the local lab.

Validation:

- `go test ./...`;
- `kustomize build config/default`;
- CRDs pass server-side dry-run against microk8s.

Checkpoint:

- inspect git status and diff;
- commit only after tests and manifest validation are green.

### Phase 2: K1s Read Client And Cluster Status

Deliverables:

- controller HTTP client;
- token and CA loading;
- bootstrap ConfigMap reader;
- `K1sCluster` reconciler;
- proxy Service EndpointSlice discovery;
- conditions and Events.

Validation:

- unit tests for auth/header/client behavior;
- envtest or fake-client tests for status conditions;
- microk8s test against `k1s-dev-a`.

Checkpoint:

- commit only after the cluster object reports expected lab status and tests pass.

### Phase 3: Exposure Resource Generation

Deliverables:

- `K1sExposure` reconciler;
- selectorless Service generation;
- managed EndpointSlice generation from k1s proxy endpoints;
- optional Ingress generation;
- ownership and cleanup.

Validation:

- unit tests for Service, EndpointSlice, and Ingress builders;
- envtest reconciliation tests;
- microk8s dry-run and live apply in `k1s-operator-dev`;
- generated Service points to the lab edge proxy endpoints.

Checkpoint:

- commit only after resource generation converges and cleanup works.

### Phase 4: App Mirroring

Deliverables:

- k1s app status parser;
- `K1sAppMirror` creation/update;
- exposure status linked to mirror state;
- readiness and app-not-found handling.

Validation:

- unit tests against representative `/status/<app>` payloads;
- failure-path tests for missing app and unavailable controller;
- live lab test with a known k1s app.

Checkpoint:

- commit only after app mirror status is accurate and failure paths are covered.

### Phase 5: Integration And Packaging

Deliverables:

- installable kustomize bundle;
- local sample manifests;
- microk8s integration runbook;
- optional WorkerBee validation wrapper if useful for repeatable local stack testing;
- release notes for v0.1.0.

Validation:

- install from clean namespace;
- create read credential Secret;
- create `K1sCluster`;
- wait for cluster readiness;
- create `K1sExposure`;
- verify Service, EndpointSlice, Ingress, mirror, status, and Events;
- curl through the Kubernetes ingress path;
- delete `K1sExposure` and verify generated resources are removed;
- delete operator install and verify CRDs/RBAC behavior is documented.

Checkpoint:

- commit and tag only after the end-to-end lab path is green.

### Stage 2: Capability CRUD And Discovery

Stage 2 deliberately increases scope. It should be developed as a checkpoint set after the Stage 1 exposure path is green in unit tests, WorkerBee local validation, k1s dev profile validation, and microk8s lab validation.

Deliverables:

- `K1sApp` CRD and reconciler for standard k1s workload/app lifecycle;
- `K1sInferenceEndpoint` CRD and reconciler for AI/ML inference endpoint lifecycle;
- `K1sResourceSet` CRD and reconciler for allowlisted native k1s manifests;
- read/write credential split in `K1sCluster.spec.authSecretRef`;
- explicit conditions for accepted, policy allowed, applied, ready, and unsupported k1s-side capability cases;
- discovery/status mapping from k1s back into Kubernetes status for standard and AI/ML workflows;
- samples for standard service, inference endpoint, and mixed resource set usage.

Validation:

- unit tests for manifest rendering, allowlist enforcement, delete policy behavior, and status parsing;
- fake-client reconciliation tests for RBAC-visible status and finalizers;
- k1s dev profile tests for standard app apply/status/delete;
- k1s-side API tests for inference resources before promoting AI/ML CRUD beyond scaffold status;
- microk8s dry-run for all CRDs/RBAC/samples;
- live microk8s run using scoped write credentials in a separate operator namespace.

Checkpoint set:

1. Stage 2A: standard `K1sApp` apply/status/delete using existing k1s deployment APIs.
2. Stage 2B: `K1sInferenceEndpoint` API scaffold and manifest rendering, marked `Unsupported` when the k1s-side inference mutation API is unavailable.
3. Stage 2C: k1s-side inference mutation/discovery API in `../k1s`, validated from workerbee local and k1s dev profile.
4. Stage 2D: resource discovery and mirror status for both standard and AI/ML resources.
5. Stage 2E: microk8s end-to-end validation with a standard service and an AI/ML inference endpoint, then stage and commit if green.

Stop conditions:

- missing or unsafe write-token boundaries;
- k1s-side inference API cannot be reconciled with the desired CRD contract;
- RBAC requires broader access than the specific CRDs, generated resources, and referenced Secrets/ConfigMaps;
- Kubernetes would need to schedule directly onto k1s to satisfy the feature.

## Acceptance Criteria For v1

v1 is complete when:

1. Users can install the operator into microk8s.
2. Users can define a `K1sCluster` for the existing `k1s-dev-a` lab without giving the operator broad admin tokens.
3. `K1sCluster.status` shows controller availability and proxy readiness.
4. Users can define a `K1sExposure` for one known k1s app.
5. The operator creates a selectorless Service, EndpointSlice, and Ingress in the exposure namespace.
6. The generated EndpointSlice contains ready proxy endpoints from `k1s-dev-a-k1s-core-ha-edge-proxy`.
7. `K1sExposure.status` reflects app readiness, proxy endpoint count, and generated resource names.
8. A `K1sAppMirror` exists and shows useful observed app state.
9. Deleting the exposure removes generated resources.
10. Failure states are visible as conditions and Events.
11. The implementation has unit tests, reconciliation tests, and a documented microk8s integration test.

## Future Work

### Gateway API

Add Gateway API support once the target Kubernetes cluster has Gateway resources installed. The likely shape is:

- keep generated Service and EndpointSlice;
- generate `HTTPRoute` instead of or alongside Ingress;
- let users bind to an existing `Gateway`;
- preserve `K1sExposure` as the user-facing object.

### API Shim Integration

Use the API shim when the operator needs Kubernetes-compatible list/watch behavior or richer projected resource state. Keep API shim credentials read-only unless a later explicitly mutating feature is designed and approved.

### k1s Route Management

Future versions may create or reconcile k1s-side ingress routes. That should be a separate capability gate because it changes the v1 read-only contract.

### L4 Exposure

L4 support should follow the k1s external-proxy stance. A future design could generate Kubernetes resources that point at an explicitly configured L4 proxy, but the operator should not silently invent a TCP/UDP load-balancing plane.

### Direct Endpoint Mirroring

Direct k1s endpoint mirroring may be useful for tightly coupled lab environments where pod or overlay IPs are routable from Kubernetes. It should remain an opt-in expert mode, not the default success path.

### Multi-Cluster And Multi-Tenant Use

Later versions can add:

- cluster-scoped install mode;
- namespace admission policies;
- allowed host/domain policies;
- explicit cross-namespace reference grants;
- per-tenant credential binding.

## Open Questions

1. Should cross-namespace `K1sCluster` references be allowed by default, or require a ReferenceGrant-like object?
2. Should `K1sExposure.spec.host` be required for Service-only exposure, or only when Ingress is enabled?
3. Should the operator refuse to expose an app when the k1s app status is not ready, or keep endpoints present and report degraded status?
4. Should generated Ingress names always be derived from the exposure name, or should users be able to specify separate Service and Ingress names?
5. Should `K1sAppMirror` be a visible CRD in v1, or an internal status-only concept until richer mirroring exists?
6. How should host ownership be validated if multiple `K1sExposure` objects claim the same host/path?
7. Should Gateway API be a v1.1 feature or wait until the local microk8s stack installs Gateway resources?

## Recommended Initial Decisions

For the first implementation pass:

1. Allow same-namespace cluster refs by default.
2. Allow cross-namespace refs only when the operator has RBAC and the user specifies `clusterRef.namespace`.
3. Require `host` when Ingress is enabled.
4. Permit Service-only exposure without `host`.
5. Keep endpoints present when the app is not ready only if the proxy is ready, but mark `Ready=False` and `AppReady=False`.
6. Create `K1sAppMirror` as a visible namespaced CRD, owned by `K1sExposure`.
7. Use Ingress for v1 because the local microk8s cluster has Ingress but not Gateway API.
8. Treat Gateway API as a planned follow-up.
