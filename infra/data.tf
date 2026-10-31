resource "google_artifact_registry_repository" "backend" {
  project       = var.project_id
  location      = var.region
  repository_id = "backend"
  format        = "DOCKER"
  description   = "kz_2601 backend images"
  depends_on    = [google_project_service.this]
}

# ---- Secret Manager: 入れ物だけ。値(version)は人が入れる ----

resource "google_secret_manager_secret" "this" {
  for_each = toset(var.secret_ids)

  project   = var.project_id
  secret_id = each.key

  replication {
    auto {}
  }

  depends_on = [google_project_service.this]
}

resource "google_secret_manager_secret_iam_member" "run_backend_accessor" {
  for_each = google_secret_manager_secret.this

  project   = var.project_id
  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.run_backend.member
}

# ---- Firestore ----

resource "google_firestore_database" "default" {
  project                     = var.project_id
  name                        = "(default)"
  location_id                 = var.region
  type                        = "FIRESTORE_NATIVE"
  concurrency_mode            = "PESSIMISTIC"
  app_engine_integration_mode = "DISABLED"

  point_in_time_recovery_enablement = var.firestore_pitr_enabled ? "POINT_IN_TIME_RECOVERY_ENABLED" : "POINT_IN_TIME_RECOVERY_DISABLED"
  delete_protection_state           = var.firestore_delete_protection ? "DELETE_PROTECTION_ENABLED" : "DELETE_PROTECTION_DISABLED"

  # terraform destroy でも DB は消さず state から外すだけ
  deletion_policy = "ABANDON"

  lifecycle {
    prevent_destroy = true
  }

  depends_on = [google_project_service.this]
}
