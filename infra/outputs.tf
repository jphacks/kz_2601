# GitHub の Environment Variables に入れる値

output "GCP_PROJECT_ID" {
  value = var.project_id
}

output "GCP_REGION" {
  value = var.region
}

output "GCP_WORKLOAD_IDENTITY_PROVIDER" {
  value = google_iam_workload_identity_pool_provider.github.name
}

output "GCP_SERVICE_ACCOUNT" {
  value = google_service_account.deployer.email
}

output "wif_condition" {
  value = local.wif_condition
}
