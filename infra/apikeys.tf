# Firebase の API キー(iOS / Web)の制限(docs/infra/credentials-inventory.md の 3)。
# キーは Firebase がアプリ登録時に作るので、Terraform は作らずに import して制限だけを持つ。
# key_string は state に入る(クライアントに配布される前提の識別子。state の閲覧は 2 人に絞る)。

resource "google_apikeys_key" "firebase" {
  for_each = var.firebase_api_keys

  project      = var.project_id
  name         = each.value.key_id
  display_name = each.value.display_name

  restrictions {
    dynamic "ios_key_restrictions" {
      for_each = length(each.value.ios_bundle_ids) > 0 ? [1] : []
      content {
        allowed_bundle_ids = each.value.ios_bundle_ids
      }
    }

    dynamic "api_targets" {
      for_each = toset(each.value.api_targets)
      content {
        service = api_targets.value
      }
    }
  }

  lifecycle {
    # Terraform からキーを消すとアプリが動かなくなる
    prevent_destroy = true
  }

  depends_on = [google_project_service.this]
}

# 既に state にあれば何もしない(stg も prod も同じ書き方で取り込む)
import {
  for_each = var.firebase_api_keys
  to       = google_apikeys_key.firebase[each.key]
  id       = "projects/${var.project_id}/locations/global/keys/${each.value.key_id}"
}
