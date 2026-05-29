# Repository Guidelines

## Project Structure & Module Organization

This repository is the Kubernetes operator for exposing selected k1s workloads to a Kubernetes cluster. `PLAN.md` is the source of truth for requirements, CRDs, reconciliation behavior, and local `microk8s`/`k1s-dev-a` lab assumptions.

Use this layout:

- `api/v1alpha1/`: CRD Go types for `K1sCluster`, `K1sExposure`, `K1sApp`, `K1sInferenceEndpoint`, `K1sResourceSet`, and `K1sAppMirror`.
- `cmd/manager/`: controller manager entrypoint.
- `internal/controller/`: reconcilers and Kubernetes resource builders.
- `internal/k1s/`: k1s controller HTTP clients for scoped read and write operations.
- `config/`: CRDs, RBAC, manager deployment, kustomize overlays, samples.
- `test/` or package-local `*_test.go`: unit, envtest, and integration tests.

## Build, Test, and Development Commands

Use these commands for the public-readiness workflow:

- `make verify`: run code generation, manifest generation, tests, vet, kustomize builds, and whitespace checks.
- `go test ./...`: run all Go tests.
- `go test ./internal/controller -run TestName`: run a focused controller test.
- `make manifests`: regenerate CRDs and RBAC from kubebuilder markers.
- `make generate`: regenerate deepcopy code.
- `kustomize build config/default`: render install manifests.
- `make dry-run`: validate rendered default manifests against the current cluster.
- `kubectl -n k1s-operator-dev get k1sclusters,k1sexposures,k1sappmirrors`: inspect operator resources in the lab namespace.

## Coding Style & Naming Conventions

Write Go in standard `gofmt` style. Keep controller-runtime patterns conventional: small reconcilers, explicit status conditions, and testable builder functions. Use responsibility-based package names such as `controller` and `k1s`.

Use Kubernetes-style API names: short, stable, explicit. Resource kinds use PascalCase (`K1sExposure`); YAML fields use lower camelCase (`clusterRef`, `pollIntervalSeconds`). Generated names must be deterministic and documented.

## Testing Guidelines

Prefer focused tests for resource builders and status-condition transitions. Add reconciliation tests for ownership, cleanup, and failure paths. Integration tests should target an isolated operator namespace. MicroK8s/k1s-dev-a validation may read k1s-dev-a bootstrap/proxy resources and may mutate k1s resources through the operator using dedicated scoped write credentials.

Name Go tests `Test<Behavior>` and fixtures after the scenario they represent, for example `exposure_proxy_service_ready.yaml`.

## Commit & Pull Request Guidelines

Use concise, imperative subjects with a scope when helpful: `docs: add operator plan`, `api: add K1sExposure types`.

Pull requests should include the problem statement, implementation summary, validation commands, and lab assumptions. Link issues when available. For operator behavior changes, include sample YAML or status output.

## Security & Configuration Tips

Do not commit tokens, kubeconfigs, or generated Secrets. The operator may call k1s mutation endpoints only with dedicated scoped write credentials stored in Kubernetes Secrets. Kubernetes users and workloads should receive RBAC to the operator CRDs, not direct k1s tokens. Keep broad HA bootstrap Secrets out of samples; use dedicated least-privilege Secret examples instead.
