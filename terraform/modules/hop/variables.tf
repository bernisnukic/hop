variable "namespace" {
  description = "Namespace hop is installed in."
  type        = string
  default     = "hop"
}

variable "image_repository" {
  type    = string
  default = "ghcr.io/bernisnukic/hop"
}

variable "image_tag" {
  description = "Image tag. Empty uses the chart's appVersion."
  type        = string
  default     = ""
}

variable "image_pull_policy" {
  type    = string
  default = "IfNotPresent"
}

variable "base_url" {
  description = "Public base URL that short links are built on."
  type        = string
  default     = "http://localhost:8080"
}

variable "postgres_instances" {
  description = "Postgres instances in the CloudNativePG cluster (1 primary + replicas)."
  type        = number
  default     = 1
}

variable "metrics_namespace" {
  description = "Namespace Prometheus runs in; the only one allowed to scrape hop."
  type        = string
  default     = "monitoring"
}

variable "values" {
  description = "Extra chart values, merged last."
  type        = any
  default     = {}
}
