# hop itself, from the chart in this repository.

locals {
  chart_path = "${path.module}/../../../charts/hop"
  # Helm only notices a local chart changed when its version does. Passing a
  # digest of the chart's files as a value makes any edit an upgrade.
  chart_digest = sha1(join("", [for f in sort(fileset(local.chart_path, "**")) : filesha1("${local.chart_path}/${f}")]))
}

resource "helm_release" "hop" {
  name             = "hop"
  namespace        = var.namespace
  create_namespace = true
  chart            = local.chart_path
  timeout          = 600
  wait             = true

  values = [
    yamlencode({
      baseURL = var.base_url
      image = {
        repository = var.image_repository
        tag        = var.image_tag
        pullPolicy = var.image_pull_policy
      }
      postgres      = { cnpg = { instances = var.postgres_instances } }
      networkPolicy = { metricsNamespace = var.metrics_namespace }
      chartDigest   = local.chart_digest
    }),
    yamlencode(var.values),
  ]
}
