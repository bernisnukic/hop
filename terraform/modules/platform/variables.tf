variable "kube_prometheus_stack_version" {
  description = "Chart version of prometheus-community/kube-prometheus-stack."
  type        = string
  default     = "93.6.1"
}

variable "cnpg_version" {
  description = "Chart version of cloudnative-pg/cloudnative-pg (the operator)."
  type        = string
  default     = "0.29.2"
}

variable "prometheus_retention" {
  description = "How long Prometheus keeps samples."
  type        = string
  default     = "7d"
}

variable "prometheus_storage_size" {
  description = "Size of Prometheus' volume. Empty keeps data on an emptyDir."
  type        = string
  default     = ""
}

variable "control_plane_metrics" {
  description = "Scrape etcd, the scheduler, the controller manager and kube-proxy. Off on kind, where they only listen on localhost."
  type        = bool
  default     = true
}
