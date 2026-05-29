# Public Early-Dev Readiness

`k1s-operator` is pre-1.0 software. The public early-dev bar is safe defaults, repeatable validation, and clear production configuration guidance.

## Current Gates

- `make verify` runs code generation, CRD/RBAC generation, unit tests, `go vet`, kustomize builds, and whitespace checks.
- CI runs `make verify` and fails if generated files drift.
- CI runs `govulncheck ./...`.
- Default manifests use namespace-scoped RBAC, localhost-bound metrics, non-root distroless runtime, an ingress-deny controller NetworkPolicy, split read/write k1s tokens, and an operator-level `K1sResourceSet` kind cap.

## Production Configuration

- Pin the operator image to a reviewed version tag or digest.
- Use HTTPS k1s controller URLs with either public trust or a namespace-local `ca.crt` key in the credentials Secret.
- Keep k1s tokens in the operator namespace and grant Kubernetes users RBAC to CRDs, not direct k1s credentials.
- Narrow `RESOURCESET_ALLOWED_KINDS` when an install should not allow all default k1s resource kinds.
- Expose metrics only through an intentional Service and NetworkPolicy.

## Deferred Release Hardening

These are expected before a stronger production release claim, but are not required for the public early-dev posture:

- published SBOMs;
- image signing;
- SLSA/provenance attestations;
- release workflow automation;
- container image vulnerability scans in CI.
