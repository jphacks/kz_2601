# IAM は *_iam_member(非権威的)だけを使う。ここに無いバインディング(サービスエージェント・
# Firebase が自動で付けるもの・人の owner)を消さないため。

resource "google_service_account" "run_backend" {
  project      = var.project_id
  account_id   = local.sa.run_backend
  display_name = "Cloud Run runtime (appconfig/ai)"
  depends_on   = [google_project_service.this]
}

resource "google_service_account" "deployer" {
  project      = var.project_id
  account_id   = local.sa.deployer
  display_name = "GitHub Actions deployer"
  depends_on   = [google_project_service.this]
}

# ---- run-backend(appconfig / ai の実行 SA) ----

resource "google_project_iam_member" "run_backend_datastore" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = google_service_account.run_backend.member
}

# ---- github-deployer(CI) ----

# --allow-unauthenticated で allUsers を付けるため run.developer では足りない
resource "google_project_iam_member" "deployer_run_admin" {
  project = var.project_id
  role    = "roles/run.admin"
  member  = google_service_account.deployer.member
}

resource "google_project_iam_member" "deployer_ar_writer" {
  project = var.project_id
  role    = "roles/artifactregistry.writer"
  member  = google_service_account.deployer.member
}

# deployer が「run-backend として」サービスをデプロイする actAs
resource "google_service_account_iam_member" "deployer_actas_run_backend" {
  service_account_id = google_service_account.run_backend.name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}

# GitHub Actions(WIF)→ deployer のなりすまし。ref の制限はプロバイダの条件側で掛ける
resource "google_service_account_iam_member" "deployer_wif" {
  service_account_id = google_service_account.deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.repository/${var.github_repository}"
}
