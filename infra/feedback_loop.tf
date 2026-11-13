# 自己改善ループの定期の実行(#368「定期的に取り込む」・docs/backend/features/feedback-loop.md)。
# 既定は feedback_loop_enabled = false で、何も作らない。true にするのは人の承認の後。
# 2 段で入れる(purge.tf と同じ):
#   1. feedback_loop_enabled = true → SA・IAM・Secret の入れ物(値は人が入れる)
#   2. Secret の値を入れ、イメージの digest を feedback_loop_job_image に入れる → ジョブと Scheduler
#
# イメージは distroless(シェルが無い)で、feedback-loop は 1 回の実行で 1 コマンドだけ。
# そこで 1 コマンド = 1 ジョブにし、Scheduler の時刻をずらして順に動かす:
#   ingest → triage → draft(1 日 1 回)、fixed と purge(1 日 1 回)。
# drafts approve(Issue の作成)は定期にしない(人の承認 1)。

# ---- 変数(この機能の分だけ。variables.tf には足さない) ----

variable "feedback_loop_enabled" {
  description = "自己改善ループの SA・IAM・Secret の入れ物・ジョブ・Scheduler を作るか。既定は false(何も作らない)"
  type        = bool
  default     = false
}

variable "feedback_loop_job_image" {
  description = "ループのジョブの初回イメージ(digest 推奨・/app/feedback-loop を含む backend のイメージ)。空ならジョブと Scheduler を作らない。作成後の更新は Terraform が追わない"
  type        = string
  default     = ""
}

variable "feedback_loop_schedules" {
  description = "コマンドごとの cron(Asia/Tokyo)。前の段の timeout より後にずらす。キーは ingest / triage / draft / fixed / purge"
  type        = map(string)
  default = {
    ingest = "0 5 * * *"
    triage = "20 5 * * *"
    draft  = "0 6 * * *"
    fixed  = "30 6 * * *"
    purge  = "50 6 * * *"
  }
  validation {
    condition     = length(setsubtract(["ingest", "triage", "draft", "fixed", "purge"], keys(var.feedback_loop_schedules))) == 0
    error_message = "feedback_loop_schedules には ingest / triage / draft / fixed / purge のすべてが要る。"
  }
}

variable "feedback_loop_paused" {
  description = "Scheduler を止めておくか(ジョブは残る・手で gcloud run jobs execute はできる)。段ごとに止めるなら loop_control/switches"
  type        = bool
  default     = false
}

variable "feedback_loop_env" {
  description = "全部のジョブに入れる秘密でない環境変数(LOOP_CONSENT_VERSIONS・LOOP_ISSUE_REPO・LOOP_SOURCE_APPSTORE など)。秘密は入れない(feedback_loop_secret_env へ)"
  type        = map(string)
  default     = {}
  validation {
    condition     = length(setintersection(keys(var.feedback_loop_env), ["LOOP_ANTHROPIC_API_KEY", "LOOP_GITHUB_TOKEN", "LOOP_ASC_PRIVATE_KEY", "LOOP_ASC_KEY_ID", "LOOP_ASC_ISSUER_ID", "LOOP_ASC_APP_ID", "APNS_AUTH_KEY"])) == 0
    error_message = "鍵・トークンは feedback_loop_env に入れない。feedback_loop_secret_env で Secret Manager の名前を指す。"
  }
  validation {
    condition     = length(setintersection(keys(var.feedback_loop_env), ["GOOGLE_CLOUD_PROJECT", "APP_ENV", "LOOP_INTAKE", "LOOP_TRIAGE", "LOOP_DRAFT", "LOOP_PUBLISH", "LOOP_NOTIFY"])) == 0
    error_message = "接続先と段のスイッチはジョブごとに決めてある(LOOP_PUBLISH は定期にしない)。feedback_loop_env には入れない。"
  }
}

variable "feedback_loop_secret_env" {
  description = "コマンドごとに Secret Manager から入れる環境変数の名前(Secret の ID = 環境変数の名前・version は latest)。LOOP_ で始まるものは入れ物をここで作る。それ以外(APNS_* など)は既にある Secret を指し、読む権限だけを付ける。値の入っていない Secret を指すとジョブの起動が失敗するので、値を入れてから足す"
  type        = map(list(string))
  default = {
    ingest = []
    triage = ["LOOP_ANTHROPIC_API_KEY"]
    draft  = []
    fixed  = ["LOOP_GITHUB_TOKEN"]
    purge  = []
  }
}

locals {
  feedback_loop_jobs_enabled = var.feedback_loop_enabled && var.feedback_loop_job_image != ""

  feedback_loop_sa = {
    job       = "feedback-loop-job"
    scheduler = "feedback-loop-scheduler"
  }

  # コマンド → その段のスイッチ(LOOP_<段>=on)・timeout・再試行
  # triage は LLM の費用がかかるので再試行しない(次の日に新しい項目と一緒に拾う)
  feedback_loop_commands = {
    ingest = { stage_env = "LOOP_INTAKE", timeout = "900s", max_retries = 1 }
    triage = { stage_env = "LOOP_TRIAGE", timeout = "1800s", max_retries = 0 }
    draft  = { stage_env = "LOOP_DRAFT", timeout = "900s", max_retries = 1 }
    fixed  = { stage_env = "LOOP_NOTIFY", timeout = "900s", max_retries = 1 }
    purge  = { stage_env = "", timeout = "1800s", max_retries = 1 }
  }

  feedback_loop_secret_ids = toset(flatten([
    for cmd, ids in var.feedback_loop_secret_env : ids if contains(keys(local.feedback_loop_commands), cmd)
  ]))
  # LOOP_ の Secret だけここで作る(APNs の鍵などは runner.tf / data.tf の入れ物を共用)
  feedback_loop_owned_secret_ids = toset([for s in local.feedback_loop_secret_ids : s if startswith(s, "LOOP_")])
}

# ---- SA ----

resource "google_service_account" "feedback_loop_job" {
  count        = var.feedback_loop_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.feedback_loop_sa.job
  display_name = "Feedback loop jobs"
  depends_on   = [google_project_service.this]
}

resource "google_service_account" "feedback_loop_scheduler" {
  count        = var.feedback_loop_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.feedback_loop_sa.scheduler
  display_name = "Feedback loop scheduler"
  depends_on   = [google_project_service.this]
}

# Firestore だけ(feedback / users の読み取りと loop_* の読み書き)。Firebase Auth・アプリの ANTHROPIC_API_KEY は持たせない
resource "google_project_iam_member" "feedback_loop_job_datastore" {
  count   = var.feedback_loop_enabled ? 1 : 0
  project = var.project_id
  role    = "roles/datastore.user"
  member  = google_service_account.feedback_loop_job[0].member
}

# deployer が「feedback-loop-job として」ジョブのイメージを更新するための actAs
resource "google_service_account_iam_member" "deployer_actas_feedback_loop_job" {
  count              = var.feedback_loop_enabled ? 1 : 0
  service_account_id = google_service_account.feedback_loop_job[0].name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}

# ---- Secret: 入れ物だけ。値は人が gcloud secrets versions add で入れる ----
# secret_ids(data.tf)に入れると run-backend も読めてしまうので、こちらに分ける。

resource "google_secret_manager_secret" "feedback_loop" {
  for_each = var.feedback_loop_enabled ? local.feedback_loop_owned_secret_ids : toset([])

  project   = var.project_id
  secret_id = each.key

  replication {
    auto {}
  }

  depends_on = [google_project_service.this]
}

# Secret ごとに付ける(プロジェクト全体の secretAccessor は付けない)
resource "google_secret_manager_secret_iam_member" "feedback_loop_accessor" {
  for_each = var.feedback_loop_enabled ? local.feedback_loop_secret_ids : toset([])

  project   = var.project_id
  secret_id = each.key
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.feedback_loop_job[0].member

  depends_on = [google_secret_manager_secret.feedback_loop]
}

# ---- ジョブ(1 コマンド = 1 ジョブ) ----

resource "google_cloud_run_v2_job" "feedback_loop" {
  for_each = { for k, v in local.feedback_loop_commands : k => v if local.feedback_loop_jobs_enabled }

  project  = var.project_id
  location = var.region
  name     = "feedback-loop-${each.key}"

  deletion_protection = true

  template {
    task_count = 1

    template {
      service_account       = google_service_account.feedback_loop_job[0].email
      max_retries           = each.value.max_retries
      timeout               = each.value.timeout
      execution_environment = "EXECUTION_ENVIRONMENT_GEN2"

      containers {
        image   = var.feedback_loop_job_image
        command = ["/app/feedback-loop"]
        args    = [each.key]

        env {
          name  = "GOOGLE_CLOUD_PROJECT"
          value = var.project_id
        }

        env {
          name  = "APP_ENV"
          value = var.env
        }

        # この段のスイッチだけを on にする(loop_control/switches が off なら、それでも止まる)
        dynamic "env" {
          for_each = toset(compact([each.value.stage_env]))
          content {
            name  = env.value
            value = "on"
          }
        }

        dynamic "env" {
          for_each = var.feedback_loop_env
          content {
            name  = env.key
            value = env.value
          }
        }

        # 秘密は名前だけ。値は Secret Manager にあり、このコードにも state にも入らない
        dynamic "env" {
          for_each = toset(try(var.feedback_loop_secret_env[each.key], []))
          content {
            name = env.value
            value_source {
              secret_key_ref {
                secret  = env.value
                version = "latest"
              }
            }
          }
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

  depends_on = [google_secret_manager_secret_iam_member.feedback_loop_accessor]
}

# Scheduler の SA は、ループのジョブにだけ run.invoker(プロジェクト全体には付けない)
resource "google_cloud_run_v2_job_iam_member" "feedback_loop_scheduler_invoker" {
  for_each = google_cloud_run_v2_job.feedback_loop

  project  = var.project_id
  location = var.region
  name     = each.value.name
  role     = "roles/run.invoker"
  member   = google_service_account.feedback_loop_scheduler[0].member
}

resource "google_cloud_scheduler_job" "feedback_loop_daily" {
  for_each = google_cloud_run_v2_job.feedback_loop

  project          = var.project_id
  region           = var.region
  name             = "feedback-loop-${each.key}-daily"
  schedule         = var.feedback_loop_schedules[each.key]
  time_zone        = "Asia/Tokyo"
  paused           = var.feedback_loop_paused
  attempt_deadline = "180s"

  # 失敗しても次の日に続きから拾う(取り込みは cursor・振り分けは新しい項目だけ)ので再試行しない
  retry_config {
    retry_count          = 0
    max_retry_duration   = "0s"
    min_backoff_duration = "5s"
    max_backoff_duration = "3600s"
    max_doublings        = 5
  }

  http_target {
    http_method = "POST"
    uri         = "https://run.googleapis.com/v2/projects/${var.project_id}/locations/${var.region}/jobs/${each.value.name}:run"

    oauth_token {
      service_account_email = google_service_account.feedback_loop_scheduler[0].email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [google_cloud_run_v2_job_iam_member.feedback_loop_scheduler_invoker]
}

output "FEEDBACK_LOOP_SERVICE_ACCOUNT" {
  description = "ループのジョブの実行 SA。feedback_loop_enabled = false なら空"
  value       = var.feedback_loop_enabled ? google_service_account.feedback_loop_job[0].email : ""
}
