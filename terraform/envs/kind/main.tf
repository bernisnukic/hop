# A three-node kind cluster with the platform and hop on it: the whole
# stack on one machine with Docker. The image is built locally and loaded
# into the nodes, so nothing has to be pushed to a registry first.
#
#   docker build -t hop:dev ../../..
#   terraform init && terraform apply

terraform {
  required_version = ">= 1.9"
  required_providers {
    kind     = { source = "tehcyx/kind", version = "~> 0.9" }
    helm     = { source = "hashicorp/helm", version = "~> 3.3" }
    external = { source = "hashicorp/external", version = "~> 2.3" }
  }
}

variable "cluster_name" {
  type    = string
  default = "hop"
}

variable "node_image" {
  type    = string
  default = "kindest/node:v1.36.4"
}

variable "image" {
  description = "Local image to load into the nodes and run."
  type        = string
  default     = "hop:dev"
}

resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = var.node_image
  wait_for_ready = true

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"
    node { role = "control-plane" }
    node { role = "worker" }
    node { role = "worker" }
  }
}

# Copy the locally built image into every node. Re-runs when the image ID changes.
data "external" "image_id" {
  program = ["sh", "-c", "printf '{\"id\":\"%s\"}' \"$(docker image inspect --format '{{.Id}}' ${var.image})\""]
}

resource "terraform_data" "load_image" {
  triggers_replace = [kind_cluster.this.id, data.external.image_id.result.id]
  provisioner "local-exec" {
    command = "kind load docker-image ${var.image} --name ${kind_cluster.this.name}"
  }
}

provider "helm" {
  kubernetes = {
    host                   = kind_cluster.this.endpoint
    client_certificate     = kind_cluster.this.client_certificate
    client_key             = kind_cluster.this.client_key
    cluster_ca_certificate = kind_cluster.this.cluster_ca_certificate
  }
}

module "platform" {
  source                = "../../modules/platform"
  control_plane_metrics = false
}

module "hop" {
  source            = "../../modules/hop"
  image_repository  = split(":", var.image)[0]
  image_tag         = split(":", var.image)[1]
  image_pull_policy = "Never"
  depends_on        = [module.platform, terraform_data.load_image]
}

output "kubeconfig_path" {
  value = kind_cluster.this.kubeconfig_path
}

output "grafana_admin_password" {
  value     = module.platform.grafana_admin_password
  sensitive = true
}
