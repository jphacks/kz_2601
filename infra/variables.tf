variable "env" {
  description = "環境名。prod または stg"
  type        = string
  validation {
    condition     = contains(["prod", "stg"], var.env)
    error_message = "env は prod か stg。"
  }
}

variable "project_id" {
  description = "GCP プロジェクト ID(プロジェクト自体は人が作る)"
  type        = string
}

variable "region" {
  description = "Cloud Run / Artifact Registry / Firestore / Scheduler のリージョン"
  type        = string
  default     = "asia-northeast1"
}

variable "apis" {
  description = "有効化する API。足すだけで、ここに無い API を止めることはしない"
  type        = list(string)
  default = [
    "artifactregistry.googleapis.com",
    "billingbudgets.googleapis.com",
    "cloudbilling.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "cloudscheduler.googleapis.com",
    "firebase.googleapis.com",
    "firestore.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "identitytoolkit.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "sts.googleapis.com",
  ]
}

variable "extra_apis" {
  description = "apis に後から足した API。prod の import(imports.prod.tf.example)は apis だけを取り込み、こちらは有効化(冪等)で揃える"
  type        = list(string)
  default = [
    "apikeys.googleapis.com",
    "firebaseappcheck.googleapis.com",
  ]
}

# ---- GitHub / WIF ----

variable "github_repository" {
  description = "WIF で受け入れるリポジトリ(owner/name)"
  type        = string
  default     = "jphacks/kz_2601"
}

variable "wif_allowed_refs" {
  description = "WIF で受け入れる ref。null なら env の既定(prod=main / stg=develop)。[] ならリポジトリだけで縛る(今の prod と同じ)"
  type        = list(string)
  default     = null
}

# ---- Secret Manager ----

variable "secret_ids" {
  description = "Secret の入れ物(値は入れない。値は人が gcloud secrets versions add で入れる)"
  type        = list(string)
  default     = ["ANTHROPIC_API_KEY"]
}

# ---- Firestore ----

variable "firestore_pitr_enabled" {
  description = "Firestore の Point-in-time recovery(7 日)"
  type        = bool
  default     = true
}

variable "firestore_delete_protection" {
  description = "Firestore の削除保護"
  type        = bool
  default     = true
}

variable "firestore_backup_retention_days" {
  description = "Firestore の日次バックアップの保持日数。0 ならスケジュールを作らない。プライバシーポリシーの消去期限と合わせる(docs/infra/firestore-backup.md)"
  type        = number
  default     = 14
  validation {
    condition     = var.firestore_backup_retention_days >= 0 && var.firestore_backup_retention_days <= 98
    error_message = "0〜98 日(最大 14 週)。"
  }
}

# ---- Firebase の API キー(apikeys.tf) ----

variable "firebase_api_keys" {
  description = "Firebase が作った API キーの制限。キーは作らず、key_id(Credentials の キー ID)で import して制限だけを持つ。空なら管理しない"
  type = map(object({
    key_id         = string
    display_name   = string
    ios_bundle_ids = optional(list(string), [])
    api_targets    = list(string)
  }))
  default = {}
}

# ---- 退会アカウントの消去ジョブ(docs/infra/account-purge.md) ----

variable "purge_enabled" {
  description = "消去ジョブ用 SA・ジョブ・スケジューラを作るか"
  type        = bool
  default     = true
}

variable "purge_job_image" {
  description = "消去ジョブの初回イメージ(digest 推奨)。空ならジョブとスケジューラを作らない(初回 CI でイメージができてから入れる)。作成後の更新は Terraform が追わない"
  type        = string
  default     = ""
}

variable "purge_job_args" {
  description = "消去ジョブの引数"
  type        = list(string)
  default     = ["-limit=50", "-older-than=696h"]
}

variable "purge_schedule" {
  description = "消去ジョブの cron(Asia/Tokyo)"
  type        = string
  default     = "30 4 * * *"
}

# ---- 予算 ----

variable "billing_account" {
  description = "課金アカウント ID(XXXXXX-XXXXXX-XXXXXX)。リポジトリには書かない"
  type        = string
}

variable "budget_display_name" {
  description = "予算の表示名"
  type        = string
}

variable "budget_amount" {
  description = "月の予算(units)。通貨は課金アカウントの通貨と一致させる"
  type        = number
}

variable "budget_currency" {
  description = "予算の通貨(課金アカウントと同じ)"
  type        = string
  default     = "JPY"
}

# ---- 監視(docs/infra/observability.md・#295) ----

variable "monitoring_enabled" {
  description = "Uptime check・ログベースの指標・アラート・ダッシュボードを作るか。prod の import で差分 0 を確かめる間は false"
  type        = bool
  default     = true
}

variable "monitored_services" {
  description = "監視する Cloud Run サービス(キーはサービス名)。host は run.app のホスト名、p95_latency_ms は p95 のアラートの閾値。空なら監視のリソースを作らない"
  type = map(object({
    host           = string
    p95_latency_ms = number
  }))
  default = {}
}

variable "alert_emails" {
  description = "アラートを送るメールアドレス(通知チャンネルを作る)。値は envs/<env>.tfvars にだけ書き、コミットしない"
  type        = list(string)
  default     = []
}

variable "alert_notification_channels" {
  description = "既にある通知チャンネル(projects/<id>/notificationChannels/<n>)。Slack はコンソールで連携してからここに足す"
  type        = list(string)
  default     = []
}

variable "uptime_check_period" {
  description = "Uptime check の間隔"
  type        = string
  default     = "300s"
  validation {
    condition     = contains(["60s", "300s", "600s", "900s"], var.uptime_check_period)
    error_message = "uptime_check_period は 60s / 300s / 600s / 900s のどれか。"
  }
}

variable "error_rate_threshold" {
  description = "5xx 率のアラートの閾値(5 分の窓での 5xx / 全リクエスト)"
  type        = number
  default     = 0.05
}

variable "claude_failure_rate_threshold" {
  description = "Claude の失敗率のアラートの閾値(10 分の窓での outcome=error / 全試行)"
  type        = number
  default     = 0.2
}

variable "claude_failure_min_count" {
  description = "Claude の失敗率のアラートを出す最小の失敗数(10 分の窓)。少ない呼び出しでの誤報を防ぐ"
  type        = number
  default     = 3
}

# ---- runner(自動の動き・runner.tf・docs/infra/runner.md) ----

variable "runner_enabled" {
  description = "runner の SA・deployer の actAs・Firestore の TTL を作るか。既定は false(何も作らない)。#288 の分離までは prod でだけ true にする"
  type        = bool
  default     = false
}

variable "runner_service_url" {
  description = "runner-prod の URL(初回デプロイの後に gcloud run services describe の status.url を入れる)。空なら Scheduler と invoker を作らない"
  type        = string
  default     = ""
  validation {
    condition     = var.runner_service_url == "" || startswith(var.runner_service_url, "https://")
    error_message = "runner_service_url は空か https:// で始まる URL。"
  }
}

variable "runner_tick_paused" {
  description = "Scheduler の runner-tick を止めておくか(止めている間は自動の動きが進まない。積まれたきっかけは残る)"
  type        = bool
  default     = false
}

variable "runner_ttl_collection_groups" {
  description = "expireAt で TTL を掛ける runner のコレクション(collection group)"
  type        = list(string)
  default     = ["done", "outbox", "rule_status"]
}

variable "runner_secret_ids" {
  description = "runner だけが読む Secret の入れ物(APNs の鍵など)。値は人が入れる。空なら作らない"
  type        = list(string)
  default     = []
}
