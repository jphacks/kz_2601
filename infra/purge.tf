# 退会アカウントの消去ジョブ(docs/infra/account-purge.md)。
# ジョブは初回だけ Terraform で作り、イメージの更新は人 / CI(gcloud run jobs update)に任せる。

locals {
  purge_job_enabled = var.purge_enabled && var.purge_job_image != ""
}

resource "google_service_account" "purge_job" {
  count        = var.purge_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.sa.purge_job
  display_name = "Account purge job"
  depends_on   = [google_project_service.this]
}

resource "google_service_account" "purge_scheduler" {
  count        = var.purge_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.sa.purge_scheduler
  display_name = "Account purge scheduler"
  depends_on   = [google_project_service.this]
}

resource "google_project_iam_member" "purge_job_datastore" {
  count   = var.purge_enabled ? 1 : 0
  project = var.project_id
  role    = "roles/datastore.user"
  member  = google_service_account.purge_job[0].member
}

resource "google_project_iam_member" "purge_job_firebaseauth" {
  count   = var.purge_enabled ? 1 : 0
  project = var.project_id
  role    = "roles/firebaseauth.admin"
  member  = google_service_account.purge_job[0].member
}

# deployer が「purge-job として」ジョブを更新するための actAs
resource "google_service_account_iam_member" "deployer_actas_purge_job" {
  count              = var.purge_enabled ? 1 : 0
  service_account_id = google_service_account.purge_job[0].name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}

resource "google_cloud_run_v2_job" "purge" {
  count    = local.purge_job_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = "account-purge"

  deletion_protection = true

  template {
    task_count = 1

    template {
      service_account       = google_service_account.purge_job[0].email
      max_retries           = 1
      timeout               = "1800s"
      execution_environment = "EXECUTION_ENVIRONMENT_GEN2"

      containers {
        image   = var.purge_job_image
        command = ["/app/purge"]
        args    = var.purge_job_args

        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project_id
        }

        resources {
          limits = {
            cpu    = "1000m"
            memory = "512Mi"
          }
        }
      }
    }
  }

  lifecycle {
    # イメージは backend の更新ごとに gcloud で差し替える。gcloud が付ける client 情報も追わない
    ignore_changes = [
      client,
      client_version,
      template[0].labels,
      template[0].annotations,
      template[0].template[0].containers[0].image,
    ]
  }
}

resource "google_cloud_run_v2_job_iam_member" "purge_scheduler_invoker" {
  count    = local.purge_job_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.purge[0].name
  role     = "roles/run.invoker"
  member   = google_service_account.purge_scheduler[0].member
}

resource "google_cloud_scheduler_job" "purge_daily" {
  count            = local.purge_job_enabled ? 1 : 0
  project          = var.project_id
  region           = var.region
  name             = "account-purge-daily"
  schedule         = var.purge_schedule
  time_zone        = "Asia/Tokyo"
  attempt_deadline = "180s"

  retry_config {
    retry_count          = 0
    max_retry_duration   = "0s"
    min_backoff_duration = "5s"
    max_backoff_duration = "3600s"
    max_doublings        = 5
  }

  http_target {
    http_method = "POST"
    uri         = "https://run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${google_cloud_run_v2_job.purge[0].name}:run"

    oauth_token {
      service_account_email = google_service_account.purge_scheduler[0].email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [google_cloud_run_v2_job_iam_member.purge_scheduler_invoker]
}
