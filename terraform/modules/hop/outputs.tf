output "namespace" {
  value = helm_release.hop.namespace
}

output "release" {
  value = helm_release.hop.name
}
