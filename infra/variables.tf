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
