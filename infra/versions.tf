terraform {
  # import ブロックの for_each(1.7+)を使う
  required_version = ">= 1.7"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.0"
    }
  }

  # bucket / prefix は envs/<env>.backend.hcl で渡す(partial configuration)。
  # GCS backend は state のロックを持つ(同時 apply は片方が待たされる)。
  backend "gcs" {}
}

provider "google" {
  project = var.project_id
  region  = var.region

  # Billing Budgets API はユーザーの ADC だと quota project が要る
  user_project_override = true
  billing_project       = var.project_id
}
