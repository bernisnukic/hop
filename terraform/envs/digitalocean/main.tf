# The same platform and app on a managed DigitalOcean Kubernetes cluster.
# CI runs `terraform validate` on this; it is not applied there, because it
# costs money. Needs DIGITALOCEAN_TOKEN in the environment.

terraform {
  required_version = ">= 1.9"
  required_providers {
    digitalocean = { source = "digitalocean/digitalocean", version = "~> 2.109" }
    helm         = { source = "hashicorp/helm", version = "~> 3.3" }
  }
  # Keep state out of the working tree, e.g. in a Spaces bucket:
  # backend "s3" { ... }
}

variable "region" {
  type    = string
  default = "fra1"
}

variable "node_size" {
  type    = string
  default = "s-2vcpu-4gb"
}

variable "image_tag" {
  description = "Tag of ghcr.io/bernisnukic/hop to run. Empty uses the chart's appVersion."
  type        = string
  default     = ""
}

variable "base_url" {
  type = string
}

data "digitalocean_kubernetes_versions" "current" {
  version_prefix = "1.34."
}

resource "digitalocean_kubernetes_cluster" "this" {
  name          = "hop"
  region        = var.region
  version       = data.digitalocean_kubernetes_versions.current.latest_version
  auto_upgrade  = true
  surge_upgrade = true

  maintenance_policy {
    day        = "sunday"
    start_time = "03:00"
  }

  node_pool {
    name       = "default"
    size       = var.node_size
    auto_scale = true
    min_nodes  = 2
    max_nodes  = 4
  }
}

provider "helm" {
  kubernetes = {
    host                   = digitalocean_kubernetes_cluster.this.endpoint
    token                  = digitalocean_kubernetes_cluster.this.kube_config[0].token
    cluster_ca_certificate = base64decode(digitalocean_kubernetes_cluster.this.kube_config[0].cluster_ca_certificate)
  }
}

module "platform" {
  source                  = "../../modules/platform"
  prometheus_retention    = "30d"
  prometheus_storage_size = "20Gi"
  # Managed control planes do not expose these to scraping.
  control_plane_metrics = false
}

module "hop" {
  source             = "../../modules/hop"
  image_tag          = var.image_tag
  base_url           = var.base_url
  postgres_instances = 2 # a primary and a hot standby
  depends_on         = [module.platform]
}

output "cluster_id" {
  value = digitalocean_kubernetes_cluster.this.id
}
