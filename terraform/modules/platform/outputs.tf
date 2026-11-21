output "grafana_admin_password" {
  description = "Password for Grafana's admin user."
  value       = random_password.grafana_admin.result
  sensitive   = true
}

output "monitoring_namespace" {
  value = helm_release.kube_prometheus_stack.namespace
}
