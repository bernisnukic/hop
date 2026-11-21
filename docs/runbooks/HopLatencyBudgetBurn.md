# HopLatencyBudgetBurn

More than 1% of redirects are taking longer than 100 ms, fast enough to spend the latency budget early.

## Check

1. **Is it everywhere or one pod:**
   ```promql
   histogram_quantile(0.99, sum by (pod, le) (rate(hop_http_request_duration_seconds_bucket{job="hop", route="GET /{code}"}[5m])))
   ```
   - **One pod:** a noisy node or CPU throttling. Compare `container_cpu_cfs_throttled_periods_total` for that pod.
   - **All pods:** the database.
2. **The database:**
   - **Row locks.** Each redirect runs `UPDATE links SET hits = hits + 1 WHERE code = $1`. A single link with a
     traffic spike makes every redirect for it wait on the same row lock.
   - **Load.** Look at the primary pod's CPU and at `pg_stat_activity`:
     ```sh
     kubectl -n hop exec -it hop-db-1 -- psql -U postgres app -c "select state, wait_event_type, count(*) from pg_stat_activity group by 1, 2"
     ```
3. **Autoscaling.** Check whether the HPA is at its maximum: `kubectl -n hop get hpa`.

## Mitigate

- **CPU-bound pods:** raise `autoscaling.maxReplicas` or the CPU request.
- **Hot-row contention:** a short-term fix is not available without a code change. The real fix is counting hits
  asynchronously (batch the increments), noted as a known limit in the README.
