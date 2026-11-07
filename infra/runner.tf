# runner(自動の動き・#341・M4)の SA・Scheduler・TTL(docs/infra/runner.md)。
# 既定は runner_enabled = false で、何も作らない。true にするのは人の承認の後。
# サービス(runner-prod)は appconfig / ai と同じく deploy-cloudrun.yml が作る(Terraform では持たない)。
# 2 段で入れる:
#   1. runner_enabled = true → SA・deployer の actAs・TTL・(鍵があれば)Secret の入れ物
#   2. 初回デプロイの後に runner_service_url を入れる → Scheduler の SA の run.invoker と runner-tick

locals {
  runner_service      = "runner-${var.env}"
  runner_service_url  = trimsuffix(var.runner_service_url, "/")
  runner_tick_enabled = var.runner_enabled && local.runner_service_url != ""

  runner_sa = {
    runner    = "runner"
    scheduler = "runner-scheduler"
  }
}

resource "google_service_account" "runner" {
  count        = var.runner_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.runner_sa.runner
  display_name = "Cloud Run runtime (runner)"
  depends_on   = [google_project_service.this]

  lifecycle {
    # docs/00_decisions.md「M4」: #288 の分離までは prod だけで動かす
    precondition {
      condition     = var.env == "prod"
      error_message = "runner は #288 の分離まで prod だけ(runner_enabled は prod でだけ true にする)。"
    }
  }
}

resource "google_service_account" "runner_scheduler" {
  count        = var.runner_enabled ? 1 : 0
  project      = var.project_id
  account_id   = local.runner_sa.scheduler
  display_name = "runner tick scheduler"
  depends_on   = [google_project_service.this]
}

# Firestore だけ。AI の鍵(ANTHROPIC_API_KEY)は持たせない(#337 の B)
resource "google_project_iam_member" "runner_datastore" {
  count   = var.runner_enabled ? 1 : 0
  project = var.project_id
  role    = "roles/datastore.user"
  member  = google_service_account.runner[0].member
}

# 他人宛ての通知の「作成から 24 時間」の判定(#339)に、送り手のアカウントの作成時刻を読む。
# cmd/runner の RUNNER_ACCOUNT_AGE=firebase。書き込みの権限は持たせない
resource "google_project_iam_member" "runner_firebaseauth_viewer" {
  count   = var.runner_enabled ? 1 : 0
  project = var.project_id
  role    = "roles/firebaseauth.viewer"
  member  = google_service_account.runner[0].member
}

# deployer が「runner として」サービスをデプロイする actAs
resource "google_service_account_iam_member" "deployer_actas_runner" {
  count              = var.runner_enabled ? 1 : 0
  service_account_id = google_service_account.runner[0].name
  role               = "roles/iam.serviceAccountUser"
  member             = google_service_account.deployer.member
}

# ---- runner だけが読む Secret(APNs の鍵など・#337「プッシュの鍵は runner だけ」) ----
# secret_ids(data.tf)に入れると run-backend も読めてしまうので、こちらに分ける。
# 入れ物だけ。値は人が gcloud secrets versions add で入れる。

resource "google_secret_manager_secret" "runner" {
  for_each = var.runner_enabled ? toset(var.runner_secret_ids) : toset([])

  project   = var.project_id
  secret_id = each.key

  replication {
    auto {}
  }

  depends_on = [google_project_service.this]
}

resource "google_secret_manager_secret_iam_member" "runner_accessor" {
  for_each = google_secret_manager_secret.runner

  project   = var.project_id
  secret_id = each.value.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.runner[0].member
}

# ---- Scheduler → POST /internal/tick(OIDC) ----

resource "google_cloud_run_v2_service_iam_member" "runner_scheduler_invoker" {
  count    = local.runner_tick_enabled ? 1 : 0
  project  = var.project_id
  location = var.region
  name     = local.runner_service
  role     = "roles/run.invoker"
  member   = google_service_account.runner_scheduler[0].member
}

resource "google_cloud_scheduler_job" "runner_tick" {
  count     = local.runner_tick_enabled ? 1 : 0
  project   = var.project_id
  region    = var.region
  name      = "runner-tick"
  schedule  = "* * * * *"
  time_zone = "Asia/Tokyo"
  paused    = var.runner_tick_paused

  # 1 回の tick は約 50 秒(cmd/runner の TickBudget)。Cloud Run の --timeout=90 と揃える
  attempt_deadline = "90s"

  # 失敗しても次の分の tick が拾う(outbox は lease で重ならない)ので再試行しない
  retry_config {
    retry_count = 0
  }

  http_target {
    http_method = "POST"
    uri         = "${local.runner_service_url}/internal/tick"

    oidc_token {
      service_account_email = google_service_account.runner_scheduler[0].email
      audience              = local.runner_service_url
    }
  }

  depends_on = [google_cloud_run_v2_service_iam_member.runner_scheduler_invoker]
}

# ---- Firestore の TTL(runner/{env}/<collection>/* の expireAt) ----
# collection は collection group(同じ名前のサブコレクション全部に効く)。
# index_config は書かない(既定の単一フィールドのインデックスを変えない)。

resource "google_firestore_field" "runner_ttl" {
  for_each = var.runner_enabled ? toset(var.runner_ttl_collection_groups) : toset([])

  project    = var.project_id
  database   = google_firestore_database.default.name
  collection = each.key
  field      = "expireAt"

  ttl_config {}
}

output "RUNNER_SERVICE_ACCOUNT" {
  description = "runner の実行 SA(deploy-cloudrun.yml の --service-account)。runner_enabled = false なら空"
  value       = var.runner_enabled ? google_service_account.runner[0].email : ""
}
