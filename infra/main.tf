data "google_project" "this" {
  project_id = var.project_id
}

locals {
  project_number = data.google_project.this.number

  default_refs = {
    prod = ["refs/heads/main"]
    stg  = ["refs/heads/develop"]
  }
  wif_refs = var.wif_allowed_refs == null ? local.default_refs[var.env] : var.wif_allowed_refs

  # refs が空なら今の prod と同じ「リポジトリだけ」の条件になる(import 直後の差分 0 用)
  wif_condition = join(" && ", compact([
    "assertion.repository == '${var.github_repository}'",
    length(local.wif_refs) == 0 ? "" : "assertion.ref in [${join(", ", [for r in local.wif_refs : "'${r}'"])}]",
  ]))

  sa = {
    run_backend     = "run-backend"
    deployer        = "github-deployer"
    purge_job       = "purge-job"
    purge_scheduler = "purge-scheduler"
  }
}

resource "google_project_service" "this" {
  for_each = toset(var.apis)

  project = var.project_id
  service = each.key

  # destroy しても API は止めない(他の利用を巻き込まない)
  disable_on_destroy         = false
  disable_dependent_services = false
}
