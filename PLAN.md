# Keleustes Operator Plan

Status: public early-development roadmap
Repository: `k1s-operator`

## Purpose

Keleustes positions k1s as a Kubernetes-adjacent runtime and edge fabric that can be consumed from an existing Kubernetes cluster. The goal is not to make k1s replace Kubernetes or make k1s nodes schedulable by Kubernetes. The goal is to let Kubernetes users safely discover selected k1s resources, request selected k1s-side capabilities, expose k1s services through Kubernetes-native resources, and observe state through Kubernetes-native status.

The operator supports both standard service workflows and AI/ML workflows:

- standard k1s `Deployment` resources through `K1sApp` and `K1sResourceSet`;
- AI/ML `InferenceCell` and `InferenceCellSet` resources through `K1sInferenceEndpoint` and `K1sResourceSet`;
- Kubernetes-side exposure through selectorless `Service`, managed `EndpointSlice`, optional `Ingress`, and optional traffic probes.

## Design Principles

1. Kubernetes remains the tenant-facing API and RBAC boundary.
2. k1s remains the runtime for resources that are uncommon or awkward in ordinary Kubernetes clusters.
3. The Kubernetes scheduler never places workloads onto k1s nodes.
4. The operator is the only component that holds scoped k1s credentials.
5. Read and write k1s credentials are separate.
6. Status, logs, labels, annotations, Events, and generated resources must never contain token values.
7. Public defaults should be conservative because the project is pre-1.0.

## Current Capabilities

### K1sCluster

`K1sCluster` describes a reachable k1s control plane, credentials, polling behavior, and the proxy endpoint used by exposure resources.

The resource reports controller availability, optional API shim availability, proxy readiness, node summary, and standard Kubernetes conditions.

### K1sApp

`K1sApp` applies and tracks one k1s standard `Deployment` manifest. It requires a scoped write token and reports accepted, applied, app-ready, and ready conditions.

### K1sExposure

`K1sExposure` creates Kubernetes-side traffic resources for a selected k1s app:

- selectorless `Service`;
- managed `EndpointSlice`;
- optional `Ingress`;
- optional traffic probe condition.

Generated traffic resources target the configured k1s proxy endpoint, not direct k1s pod or replica IPs.

### K1sInferenceEndpoint

`K1sInferenceEndpoint` builds and applies a k1s `InferenceCell` or `InferenceCellSet` for AI/ML inference workflows. It reports readiness, phase, API endpoint, active executor, and last error without exposing credentials.

### K1sResourceSet

`K1sResourceSet` applies, prunes, and deletes a managed set of selected k1s resources. Public defaults cap managed kinds to `Deployment`, `InferenceCell`, and `InferenceCellSet`. Per-resource `spec.allowedKinds` can narrow the operator cap but cannot widen it.

## Security Requirements

1. The default install uses namespace-scoped RBAC.
2. The default install does not create a `ClusterRole` or `ClusterRoleBinding`.
3. k1s read and write tokens live in namespace-local Kubernetes Secrets.
4. Mutation resources require a scoped k1s write token.
5. Read-only installs can omit the write token.
6. HTTPS k1s controller URLs should be used for production installs.
7. Private k1s controller CAs are configured through the credentials Secret `ca.crt` key.
8. Cross-namespace proxy Service reads require explicit Kubernetes RBAC in the target namespace.
9. Metrics bind to localhost by default.
10. The controller pod runs as non-root, with a read-only root filesystem, dropped capabilities, and `RuntimeDefault` seccomp.
11. The default install includes an ingress-deny NetworkPolicy for the controller pod.

## Public Install Defaults

The public default kustomize install should remain suitable for early-development publication:

- namespace: `k1s-operator-system`;
- controller image tag: pre-1.0 development tag until a release workflow publishes immutable artifacts;
- `WATCH_NAMESPACE` populated from the operator pod namespace;
- `RESOURCESET_ALLOWED_KINDS=Deployment,InferenceCell,InferenceCellSet`;
- metrics bound to `127.0.0.1:8080`;
- no metrics Service by default;
- no cluster-scoped RBAC by default.

Production users should pin the controller image to a reviewed tag or digest, use HTTPS k1s controller URLs, and grant tenant RBAC only to the CRDs they are allowed to manage.

## Validation Gates

Every checkpoint should pass:

- `make verify`;
- kustomize build for default install and samples;
- Kubernetes server-side dry-run for default install;
- `govulncheck ./...`;
- focused unit tests for controller behavior and policy boundaries.

Runtime validation should cover:

- operator rollout;
- `K1sCluster` readiness;
- standard service apply/status through `K1sApp`;
- Kubernetes-side exposure through `K1sExposure`;
- AI/ML inference apply/status through `K1sInferenceEndpoint`;
- mixed standard plus AI/ML management through `K1sResourceSet`;
- a negative `K1sResourceSet` test proving a tenant cannot widen the operator kind cap.

## Roadmap

### Stage 1: Exposure Bridge

Represent a reachable k1s control plane, mirror selected app status, and expose selected k1s apps through Kubernetes `Service`, `EndpointSlice`, and optional `Ingress`.

### Stage 2: Capability Delegation

Allow authorized Kubernetes users and workloads to create, discover, update, and delete selected k1s-side resources through operator-owned CRDs and normal Kubernetes RBAC. This stage includes both standard services and AI/ML inference resources.

### Stage 3: Policy And Multi-Tenancy

Add stronger namespace policy controls, reference grants, allowed host/domain policies, and richer tenant separation without moving k1s credentials into tenant workloads.

### Stage 4: Release Hardening

Add SBOM publication, image signing, provenance attestations, release automation, and container image vulnerability scans before making stronger production-release claims.

## Non-Goals

- Replacing k1s desired-state management with Kubernetes as the sole control plane.
- Making k1s nodes Kubernetes-schedulable.
- Letting the Kubernetes controller manager or scheduler directly place workloads on k1s.
- Mirroring k1s pod IPs as the default traffic path.
- Assuming k1s overlay, WireGuard, proxy, or pod networks are directly reachable from Kubernetes.
- General TCP/UDP load-balancer bridging in the initial API.
- Installing Gateway API or owning DNS records by default.
