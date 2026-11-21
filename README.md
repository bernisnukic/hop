# hop

[![ci](https://github.com/bernisnukic/hop/actions/workflows/ci.yml/badge.svg)](https://github.com/bernisnukic/hop/actions/workflows/ci.yml) [![e2e](https://github.com/bernisnukic/hop/actions/workflows/e2e.yml/badge.svg)](https://github.com/bernisnukic/hop/actions/workflows/e2e.yml)

A small link shortener in Go, and everything needed to run it properly on Kubernetes: Terraform for the cluster
and its add-ons, a Helm chart, Postgres through an operator, SLOs with burn-rate alerts, a Grafana dashboard and
runbooks. One script stands the whole stack up on a local kind cluster and checks that it behaves as designed.

The app is deliberately small. The repository is about operating it.

```
                       ┌────────────── kind cluster (Terraform) ──────────────┐
                       │                                                       │
 POST /api/links ───►  │  Service hop ──► hop pods (2-6, HPA) ──► Postgres     │
 GET  /{code}    ◄───  │                    │  :9090/metrics       (CloudNativePG)
                       │                    ▼                                  │
                       │  Prometheus ── SLO rules ──► Alertmanager (page/ticket)
                       │      └──────► Grafana dashboard                       │
                       └───────────────────────────────────────────────────────┘
```

## What's here

| Path | What it is |
|---|---|
| `cmd/hop`, `internal/` | The service: `net/http`, pgx, Prometheus metrics, embedded SQL migrations |
| `charts/hop` | Helm chart: Deployment, Service, HPA, PDB, NetworkPolicy, CloudNativePG `Cluster`, ServiceMonitor, PrometheusRule, dashboard, `helm test` |
| `charts/hop/files/slo-rules.yaml` | The SLO recording rules and alerts, the single source for the chart and the tests |
| `slo/tests.yaml` | promtool unit tests for those alerts |
| `terraform/modules/platform` | kube-prometheus-stack and the CloudNativePG operator, with Alertmanager routing |
| `terraform/modules/hop` | The chart as a Terraform release |
| `terraform/envs/kind` | A three-node kind cluster with everything on it |
| `terraform/envs/digitalocean` | The same modules on managed Kubernetes (validated in CI, not applied) |
| `scripts/e2e.sh` | Builds, applies and runs the end-to-end checks |
| `docs/slos.md`, `docs/runbooks/` | Why the objectives are what they are, and what to do when an alert fires |

## Run it

You need Docker, kind, kubectl, Helm and Terraform.

```sh
make e2e     # build the image, terraform apply the kind environment, run the checks
make down    # delete the cluster
```

Then look around:

```sh
export KUBECONFIG=$(terraform -chdir=terraform/envs/kind output -raw kubeconfig_path)
kubectl -n hop port-forward svc/hop 8080:80 &
curl -s -X POST localhost:8080/api/links -d '{"url":"https://example.com"}'

kubectl -n monitoring port-forward svc/kube-prometheus-stack-grafana 3000:80 &
terraform -chdir=terraform/envs/kind output -raw grafana_admin_password   # user: admin
```

Without Kubernetes, `make run` starts the service with an in-memory store.

## What the end-to-end run checks

`scripts/e2e.sh` runs in CI on every push (`.github/workflows/e2e.yml`) and fails if any of these is not true:

1. Terraform creates the cluster, installs the platform and deploys hop, with every release reporting ready.
2. `helm test` creates a link through the Service and follows the redirect.
3. 200 redirects are counted in Postgres.
4. Prometheus scrapes both pods, and the SLO recording rules return values: a 0 error ratio and redirects under
   100 ms.
5. No hop alert is firing.
6. **A rolling restart under load loses nothing:** 600 requests sent from inside the cluster during
   `kubectl rollout restart` all come back as redirects.
7. **The NetworkPolicy holds:** a pod outside the monitoring namespace cannot read `/metrics`.

## Decisions worth knowing about

- **Shutdown drains first.** On SIGTERM, `/readyz` starts failing and the pod keeps serving for
  `drainDelaySeconds`, so it leaves the Service's endpoints before it stops accepting connections. Together with
  `maxUnavailable: 0`, this is what makes check 6 pass.
- **Liveness and readiness check different things.** Liveness only asks whether the process serves HTTP. Readiness
  also needs migrations done and the database answering. A database outage takes pods out of rotation instead of
  restarting them in a loop.
- **Migrations run in the app, under a Postgres advisory lock.** Replicas starting together apply each migration
  once (tested with five concurrent migrators against a real Postgres in CI). Until the database is reachable, the
  app retries with backoff while liveness stays green.
- **Metric labels use the route pattern** (`GET /{code}`), never the raw path, so a scan of random URLs cannot
  blow up the series count.
- **The SLOs are tested like code.** promtool checks that a 10% error rate pages, 0.1% stays silent, and slow
  redirects page or open a ticket depending on how slow and for how long. See [docs/slos.md](docs/slos.md).
- **Least privilege by default.** The image is distroless and runs as non-root with a read-only root filesystem and
  no capabilities. No service account token is mounted. Egress is limited to DNS and the app's own database.

## Known limits

- **Hit counting.** Every redirect updates one row, so a single very popular link serialises on that row's lock.
  Batching the increments would fix it; the latency runbook covers how this shows up.
- **The kind environment runs one Postgres instance with no backups.** On DigitalOcean the module runs two
  instances. Backups to object storage through CloudNativePG's Barman support would be the next step.
- **No ingress or TLS on kind.** The chart has an optional Ingress, so a real cluster would add an ingress
  controller and cert-manager. That is also where an availability check from outside would go; see the last
  section of [docs/slos.md](docs/slos.md).

## License

MIT
