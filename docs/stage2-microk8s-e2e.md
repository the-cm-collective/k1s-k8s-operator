# Stage 2 MicroK8s End-To-End Validation

Status: validated on 2026-05-28.

This checkpoint validates the Stage 2 capability bridge with MicroK8s as the Kubernetes API surface and a k1s controller profile from the sibling `../k1s` checkout. It covers both standard service workflows and AI/ML inference workflows without letting Kubernetes schedule onto k1s nodes.

## Topology

- Kubernetes API: local MicroK8s.
- Operator mode: out-of-cluster manager using the MicroK8s kubeconfig.
- k1s runtime: WorkerBee `k1s-dev-min-sqlite` profile built from `../k1s`.
- k1s controller URL: profile-local controller HTTP URL.
- k1s credentials: read and write tokens stored in a namespace-local Secret as `controllerReadToken` and `controllerWriteToken`.

The out-of-cluster manager path is used when the WorkerBee profile controller is bound to host loopback. For an in-cluster operator deployment, use a controller URL reachable from pods, such as the MicroK8s k1s stack Service.

## Kubernetes Resources

Use an isolated namespace, for example `k1s-operator-stage2e`, with these resources:

- `K1sCluster` with a scoped `authSecretRef` and an `External` proxy target.
- `K1sApp` containing a native k1s `Deployment` manifest for a standard HTTP service.
- `K1sInferenceEndpoint` containing model, executor, fabric, and member intent for an AI/ML inference cell.

The operator must hold the k1s write token. Kubernetes workloads and users should only receive Kubernetes RBAC for the CRDs they are allowed to create or update.

## Validation Gates

Run these gates before the checkpoint is considered green:

1. Start MicroK8s and confirm the API is ready.
2. Start or validate a WorkerBee k1s profile from `../k1s`.
3. Apply operator CRDs to MicroK8s.
4. Create a namespace-local Secret with separate read and write token keys.
5. Start the operator manager.
6. Apply `K1sCluster`, `K1sApp`, and `K1sInferenceEndpoint`.
7. Wait for:
   - `K1sCluster` `Ready=True`;
   - `K1sApp.status.ready=true`;
   - `K1sInferenceEndpoint.status.phase=READY`;
   - `K1sInferenceEndpoint.status.activeExecutor=ray`.
8. Query k1s directly with the read token and verify:
   - `GET /status/<app>?details=1` returns `200` and ready status;
   - `GET /inference/cells/<namespace>/<name>` returns `200` and `READY`.
9. Delete the `K1sApp` and `K1sInferenceEndpoint`.
10. Query k1s directly again and verify both resources return `404`.
11. Stop the local manager and WorkerBee profile, then remove the test namespace and CRDs.

## 2026-05-28 Result

The Stage 2E run passed:

- standard service `K1sApp/stage2e-echo` reached ready status;
- AI/ML `K1sInferenceEndpoint/stage2e-cell` reached `READY` with executor `ray`;
- direct k1s discovery returned `200` for both resources before deletion;
- operator finalizers deleted both k1s-side resources;
- direct k1s discovery returned `404` for both resources after deletion.
