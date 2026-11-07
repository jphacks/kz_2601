# 通報のアラート(#321・docs/infra/moderation.md#通報のアラート)。
# 通知チャンネルと有効/無効は monitoring.tf と同じ(local.monitoring・local.notification_channels)。
#
# 元のログ(どちらも uid と内容は出さない):
#   通報:     backend/internal/appconfig/moderation.go の slog.Warn("moderation: report", reportId, kind, targetId, reason)
#   自動停止: backend/internal/runner/stops.go の "runner.stop"(event=runner.stop, by=auto, level=notify, app_id)
# httpx.NewCloudHandler が msg を message に変えるので、フィルタは jsonPayload.message を見る。

locals {
  moderation_runbook_url = "https://github.com/jphacks/kz_2601/blob/develop/docs/infra/moderation.md"
}

# ---- ログベースの指標 ----

# POST /v1/reports で新しい通報ができた 1 回(同じ人の同じ対象への 2 回目は created=false でログが出ない)
resource "google_logging_metric" "moderation_reports" {
  count = local.monitoring ? 1 : 0

  project     = var.project_id
  name        = "moderation_reports"
  description = "New user reports from POST /v1/reports (#321). kind=target type, reason=report reason"
  filter      = "resource.type=\"cloud_run_revision\" AND jsonPayload.message=\"moderation: report\""

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
    labels {
      key        = "kind"
      value_type = "STRING"
    }
    labels {
      key        = "reason"
      value_type = "STRING"
    }
  }

  label_extractors = {
    kind   = "EXTRACT(jsonPayload.kind)"
    reason = "EXTRACT(jsonPayload.reason)"
  }

  depends_on = [google_project_service.monitoring]
}

# runner が止めを書いた 1 回(今は通報による通知の自動一時停止だけ・#339 / #346)
resource "google_logging_metric" "runner_stops" {
  count = local.monitoring ? 1 : 0

  project     = var.project_id
  name        = "runner_stops"
  description = "Stops written by the runner, e.g. the automatic notify pause on reports (#339). by=auto|operator"
  filter      = "resource.type=\"cloud_run_revision\" AND jsonPayload.event=\"runner.stop\""

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
    labels {
      key        = "by"
      value_type = "STRING"
    }
    labels {
      key        = "level"
      value_type = "STRING"
    }
  }

  label_extractors = {
    by    = "EXTRACT(jsonPayload.by)"
    level = "EXTRACT(jsonPayload.level)"
  }

  depends_on = [google_project_service.monitoring]
}

# ---- アラート ----

# 今は件数が少ないので 1 件でも知らせる。増えたら threshold_value を上げる
resource "google_monitoring_alert_policy" "moderation_reports" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] user report received"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "report count > 0 (5 min window)"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/moderation_reports\" AND ${local.service_filter}"
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.label.service_name"]
      }
      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
  documentation {
    mime_type = "text/markdown"
    content   = "通報が来た。`moderate reports` で開いている通報を見て、24 時間以内に対応する。Runbook: ${local.moderation_runbook_url}#通報を受けたら運用"
  }

  depends_on = [google_logging_metric.moderation_reports]
}

# 24 時間で 3 人の通報があったアプリの通知を runner が自動で止めた
resource "google_monitoring_alert_policy" "moderation_auto_pause" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] app notifications auto-paused on reports"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "runner.stop by=auto > 0 (5 min window)"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/runner_stops\" AND metric.labels.by = \"auto\""
      comparison      = "COMPARISON_GT"
      threshold_value = 0
      duration        = "0s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.label.service_name"]
      }
      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channels
  alert_strategy {
    auto_close = "1800s"
  }
  documentation {
    mime_type = "text/markdown"
    content   = "通報が重なったアプリの通知を runner が自動で一時停止した。`moderate stops` で止まっているアプリを見て、通報の中身を確かめる(続けて止める / 解除する)。Runbook: ${local.moderation_runbook_url}#通報を受けたら運用"
  }

  depends_on = [google_logging_metric.runner_stops]
}
