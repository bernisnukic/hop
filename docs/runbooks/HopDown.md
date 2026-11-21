# HopDown

Prometheus has not scraped any hop pod for 5 minutes. Either no pod is running, or Prometheus cannot reach them.

## Check

1. **Are pods running?**
   ```sh
   kubectl -n hop get deploy,pods -l app.kubernetes.io/name=hop
   kubectl -n hop describe pods -l app.kubernetes.io/name=hop | tail -40
   ```
   Look for `ImagePullBackOff`, `CrashLoopBackOff`, or pods stuck `Pending` (no capacity).
2. **If pods are running, is it the scrape?** Look at the target in Prometheus (Status → Targets, job `hop`). A
   `context deadline exceeded` there usually means the NetworkPolicy no longer lets the monitoring namespace in.
   Check `networkPolicy.metricsNamespace` matches where Prometheus runs.
3. **Did the ServiceMonitor go away?** `kubectl -n hop get servicemonitor`.

## Mitigate

- **A bad image or config:** `helm rollback hop -n hop`.
- **No capacity:** scale the node pool or lower `resources.requests`.
