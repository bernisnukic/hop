# HopErrorBudgetBurn

hop is returning 5xx responses fast enough to spend its availability error budget early. The `long_window` label
says how fast: `1h` and `6h` page, while `1d` and `3d` open a ticket.

## Check

1. **Which route is failing**, in Prometheus or the hop dashboard:
   ```promql
   sum by (route, code) (rate(hop_http_requests_total{job="hop", code=~"5.."}[5m]))
   ```
2. **The logs.** Every 5xx is logged with its cause:
   ```sh
   kubectl -n hop logs -l app.kubernetes.io/name=hop --since=15m | grep '"level":"ERROR"'
   ```
3. **The database.** Almost every 5xx path in hop is a failed query.
   ```sh
   kubectl -n hop get cluster.postgresql.cnpg.io
   kubectl -n hop get pods -l cnpg.io/cluster=hop-db
   ```
4. **A recent rollout:**
   ```sh
   kubectl -n hop rollout history deploy/hop
   ```

## Mitigate

- **A rollout started it:** run `kubectl -n hop rollout undo deploy/hop`, or `helm rollback hop -n hop`.
- **The database is failing over or down:** CloudNativePG promotes a replica on its own when one exists. With
  `instances: 1` there is no replica, so check the primary pod's events and volume.
- **Connection limits** (`too many clients` in the logs): reduce replicas, or raise `max_connections` in the Cluster.

## Afterwards

Note how much budget the incident used (the dashboard's "Availability budget left" panel). If it is close to zero,
stop risky deploys until it recovers.
