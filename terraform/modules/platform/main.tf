# The cluster add-ons hop relies on: the Prometheus Operator stack
# (Prometheus, Alertmanager, Grafana, kube-state-metrics, node-exporter)
# and the CloudNativePG operator for Postgres.

resource "random_password" "grafana_admin" {
  length  = 24
  special = false
}

resource "helm_release" "kube_prometheus_stack" {
  name             = "kube-prometheus-stack"
  namespace        = "monitoring"
  create_namespace = true
  repository       = "https://prometheus-community.github.io/helm-charts"
  chart            = "kube-prometheus-stack"
  version          = var.kube_prometheus_stack_version
  timeout          = 900
  wait             = true

  values = [yamlencode({
    prometheus = {
      prometheusSpec = {
        # Pick up ServiceMonitors and PrometheusRules from every release,
        # not only ones labelled release=kube-prometheus-stack.
        serviceMonitorSelectorNilUsesHelmValues = false
        podMonitorSelectorNilUsesHelmValues     = false
        ruleSelectorNilUsesHelmValues           = false
        retention                               = var.prometheus_retention
        resources = {
          requests = { cpu = "100m", memory = "400Mi" }
          limits   = { memory = "1500Mi" }
        }
        storageSpec = var.prometheus_storage_size == "" ? null : {
          volumeClaimTemplate = {
            spec = {
              accessModes = ["ReadWriteOnce"]
              resources   = { requests = { storage = var.prometheus_storage_size } }
            }
          }
        }
      }
    }

    alertmanager = {
      config = {
        route = {
          receiver        = "ticket"
          group_by        = ["alertname", "slo"]
          group_wait      = "30s"
          group_interval  = "5m"
          repeat_interval = "4h"
          routes = [
            # Wire "page" to PagerDuty/Opsgenie and "ticket" to a chat channel
            # or issue tracker. Both are empty receivers here.
            { receiver = "page", matchers = ["severity=\"page\""] },
            { receiver = "null", matchers = ["alertname=~\"Watchdog|InfoInhibitor\""] },
          ]
        }
        inhibit_rules = [{
          # A page for an SLO hides the ticket for the same SLO.
          source_matchers = ["severity=\"page\""]
          target_matchers = ["severity=\"ticket\""]
          equal           = ["alertname", "slo"]
        }]
        receivers = [{ name = "page" }, { name = "ticket" }, { name = "null" }]
      }
    }

    grafana = {
      adminPassword = random_password.grafana_admin.result
      sidecar = {
        dashboards = { enabled = true, label = "grafana_dashboard", searchNamespace = "ALL" }
      }
    }

    kubeEtcd              = { enabled = var.control_plane_metrics }
    kubeControllerManager = { enabled = var.control_plane_metrics }
    kubeScheduler         = { enabled = var.control_plane_metrics }
    kubeProxy             = { enabled = var.control_plane_metrics }
  })]
}

resource "helm_release" "cnpg" {
  name             = "cnpg"
  namespace        = "cnpg-system"
  create_namespace = true
  repository       = "https://cloudnative-pg.github.io/charts"
  chart            = "cloudnative-pg"
  version          = var.cnpg_version
  wait             = true
}
