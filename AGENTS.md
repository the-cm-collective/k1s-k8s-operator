# Repository Guidelines

## Project Structure & Module Organization

This repository is the planned Kubernetes operator for exposing selected k1s workloads to a Kubernetes cluster. `PLAN.md` is the source of truth for requirements, CRDs, reconciliation behavior, and local `microk8s`/`k1s-dev-a` lab assumptions.

As implementation lands, use this layout:

- `api/v1alpha1/`: CRD Go types for `K1sCluster`, `K1sExposure`, and `K1sAppMirror`.
- `cmd/manager/`: controller manager entrypoint.
- `internal/controller/`: reconcilers and Kubernetes resource builders.
- `internal/k1s/`: read-only k1s HTTP/API shim clients.
- `config/`: CRDs, RBAC, manager deployment, kustomize overlays, samples.
- `test/` or package-local `*_test.go`: unit, envtest, and integration tests.

## Build, Test, and Development Commands

Use these commands once the Go scaffold exists:

- `go test ./...`: run all Go tests.
- `go test ./internal/controller -run TestName`: run a focused controller test.
- `kustomize build config/default`: render install manifests.
- `kubectl apply --dry-run=server -k config/default`: validate manifests against the cluster.
- `kubectl -n k1s-operator-dev get k1sclusters,k1sexposures,k1sappmirrors`: inspect operator resources in the lab namespace.

Until implementation exists, validate docs with `git diff --check` and Markdown review.

## Coding Style & Naming Conventions

Write Go in standard `gofmt` style. Keep controller-runtime patterns conventional: small reconcilers, explicit status conditions, and testable builder functions. Use responsibility-based package names such as `controller` and `k1s`.

Use Kubernetes-style API names: short, stable, explicit. Resource kinds use PascalCase (`K1sExposure`); YAML fields use lower camelCase (`clusterRef`, `pollIntervalSeconds`). Generated names must be deterministic and documented.

## Testing Guidelines

Prefer focused tests for resource builders and status-condition transitions. Add reconciliation tests for ownership, cleanup, and failure paths. Integration tests should target `k1s-operator-dev` and must not mutate `k1s-dev-a` except by reading Services, EndpointSlices, ConfigMaps, and status APIs.

Name Go tests `Test<Behavior>` and fixtures after the scenario they represent, for example `exposure_proxy_service_ready.yaml`.

## Commit & Pull Request Guidelines

This repository has no commit history yet. Use concise, imperative subjects with a scope when helpful: `docs: add operator plan`, `api: add K1sExposure types`.

Pull requests should include the problem statement, implementation summary, validation commands, and lab assumptions. Link issues when available. For operator behavior changes, include sample YAML or status output.

## Security & Configuration Tips

Do not commit tokens, kubeconfigs, or generated Secrets. The v1 operator must use read-only k1s credentials and must not call k1s mutation endpoints. Keep broad HA bootstrap Secrets out of samples; use dedicated least-privilege Secret examples instead.
