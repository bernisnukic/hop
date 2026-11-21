# SLOs

hop has two service level objectives. The rules live in
[`charts/hop/files/slo-rules.yaml`](../charts/hop/files/slo-rules.yaml) and are unit-tested in
[`slo/tests.yaml`](../slo/tests.yaml).

| SLO | Indicator | Objective | Error budget (30 days) |
|---|---|---|---|
| Availability | Requests that are not 5xx, out of all requests except `/healthz` and `/readyz` | 99.5% | 0.5%, about 3.6 hours of total outage |
| Latency | Redirects (`GET /{code}`) served in under 100 ms | 99% | 1% of redirects may be slower |

## Why these numbers

- **Health checks are excluded.** Probes run every 2 seconds on every pod and almost never fail. Counting them
  would hide real failures behind thousands of easy successes.
- **Latency is measured on redirects only.** A redirect is what a person clicking a link waits for. Creating a link
  is rarer and can be slower without anyone noticing.
- **100 ms is a histogram bucket boundary.** The SLI is computed exactly from the `le="0.1"` bucket rather than
  estimated with `histogram_quantile`, so the code keeps 0.1 in its bucket list.
- **99.5%, not 99.9%.** With one Postgres primary, a failover or a node drain can cost minutes. 99.9% would be
  spent by one bad afternoon and the alerts would page for things nobody could act on.

## Alerting

Each SLO has four alerts, following the multiwindow, multi-burn-rate approach from the Google SRE workbook. A
burn rate of 1 spends the budget exactly over 30 days.

| Severity | Burn rate | Long window | Short window | Budget spent before it fires |
|---|---|---|---|---|
| page | 14.4 | 1 hour | 5 minutes | 2% |
| page | 6 | 6 hours | 30 minutes | 5% |
| ticket | 3 | 1 day | 2 hours | 10% |
| ticket | 1 | 3 days | 6 hours | 10% |

- **The long window** shows the budget is really burning, not just a blip.
- **The short window** (a twelfth of the long one) shows it is still burning, so the alert clears minutes after the
  problem does, not hours.
- **Alertmanager inhibits** a ticket while a page for the same SLO is firing.

`HopDown` pages separately when Prometheus cannot scrape any hop pod for 5 minutes. With nothing scraped, the burn
rates have no data and could not fire.

## What the SLOs do not see

If every pod is unready, the Service has no endpoints and requests fail before reaching hop, so hop never counts
them. `HopDown` does not catch that either: the pods are still running and still scraped. In production, an
ingress-level or synthetic availability check would sit next to these. On a single kind cluster there is no
ingress in front, so this repository does not have one.
