# 監視(#295・docs/infra/observability.md)。
# Uptime check(/health)・ログベースの指標・アラート・ダッシュボード 1 枚。
# Error Reporting の通知はコンソールで設定する(Terraform のリソースが無い)。

locals {
  monitoring = var.monitoring_enabled && length(var.monitored_services) > 0

  # 監視するサービスだけに絞るフィルタ
  service_filter = "resource.labels.service_name = one_of(${join(", ", [for s in sort(keys(var.monitored_services)) : "\"${s}\""])})"

  notification_channels = concat(
    [for c in google_monitoring_notification_channel.email : c.name],
    var.alert_notification_channels,
  )

  runbook_url = "https://github.com/jphacks/kz_2601/blob/develop/docs/infra/observability.md"

  request_count_filter = "resource.type = \"cloud_run_revision\" AND metric.type = \"run.googleapis.com/request_count\" AND ${local.service_filter}"
  claude_calls_filter  = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/ai_claude_calls\" AND ${local.service_filter}"
}

resource "google_project_service" "monitoring" {
  for_each = local.monitoring ? toset(["logging.googleapis.com", "monitoring.googleapis.com"]) : toset([])

  project = var.project_id
  service = each.key

  disable_on_destroy         = false
  disable_dependent_services = false
}

# ---- 通知チャンネル ----

resource "google_monitoring_notification_channel" "email" {
  for_each = local.monitoring ? toset(var.alert_emails) : toset([])

  project      = var.project_id
  display_name = "backend alerts (${var.project_id}): ${each.key}"
  type         = "email"
  labels = {
    email_address = each.key
  }

  depends_on = [google_project_service.monitoring]
}

# ---- ログベースの指標 ----

# ai.usage(#298・backend/internal/ai/usage_log.go)の 1 行 = Claude への HTTP 試行 1 回
resource "google_logging_metric" "claude_calls" {
  count = local.monitoring ? 1 : 0

  project     = var.project_id
  name        = "ai_claude_calls"
  description = "Claude Messages API attempts from ai.usage logs (#295). outcome=ok|refusal|error"
  filter      = "resource.type=\"cloud_run_revision\" AND jsonPayload.event=\"ai.usage\""

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
    labels {
      key        = "outcome"
      value_type = "STRING"
    }
    labels {
      key        = "operation"
      value_type = "STRING"
    }
    labels {
      key        = "error_kind"
      value_type = "STRING"
    }
  }

  label_extractors = {
    outcome    = "EXTRACT(jsonPayload.outcome)"
    operation  = "EXTRACT(jsonPayload.operation)"
    error_kind = "EXTRACT(jsonPayload.error_kind)"
  }

  depends_on = [google_project_service.monitoring]
}

# httpx.Recover の "panic recovered"(Error Reporting にも行く)
resource "google_logging_metric" "panics" {
  count = local.monitoring ? 1 : 0

  project     = var.project_id
  name        = "backend_panics"
  description = "Recovered panics in the Go backends (#295)"
  filter      = "resource.type=\"cloud_run_revision\" AND jsonPayload.message=\"panic recovered\""

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }

  depends_on = [google_project_service.monitoring]
}

# ---- Uptime check ----

resource "google_monitoring_uptime_check_config" "health" {
  for_each = local.monitoring ? var.monitored_services : {}

  project      = var.project_id
  display_name = "${each.key} /health"
  timeout      = "10s"
  period       = var.uptime_check_period

  http_check {
    path           = "/health"
    port           = 443
    use_ssl        = true
    validate_ssl   = true
    request_method = "GET"
    accepted_response_status_codes {
      status_class = "STATUS_CLASS_2XX"
    }
  }

  content_matchers {
    content = "ok"
    matcher = "CONTAINS_STRING"
  }

  monitored_resource {
    type = "uptime_url"
    labels = {
      project_id = var.project_id
      host       = each.value.host
    }
  }

  depends_on = [google_project_service.monitoring]
}

# ---- アラート ----

resource "google_monitoring_alert_policy" "uptime" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] backend /health uptime check failing"
  combiner     = "OR"
  severity     = "CRITICAL"

  conditions {
    display_name = "/health fails from 2+ regions"
    condition_threshold {
      filter          = "resource.type = \"uptime_url\" AND metric.type = \"monitoring.googleapis.com/uptime_check/check_passed\" AND metric.labels.check_id = one_of(${join(", ", [for c in google_monitoring_uptime_check_config.health : "\"${c.uptime_check_id}\""])})"
      comparison      = "COMPARISON_GT"
      threshold_value = 1
      duration        = "60s"
      aggregations {
        alignment_period     = "600s"
        per_series_aligner   = "ALIGN_NEXT_OLDER"
        cross_series_reducer = "REDUCE_COUNT_FALSE"
        group_by_fields      = ["resource.label.host"]
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
    content   = "/health が 2 か所以上のリージョンから失敗。Runbook: ${local.runbook_url}#uptime-check"
  }
}

# 5xx 率: Cloud Run の request_count(GFE が返した 5xx も含む)
resource "google_monitoring_alert_policy" "http_5xx_rate" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] backend 5xx rate > ${var.error_rate_threshold * 100}%"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "5xx / all requests (5 min window)"
    condition_threshold {
      filter             = "${local.request_count_filter} AND metric.labels.response_code_class = \"5xx\""
      denominator_filter = local.request_count_filter
      comparison         = "COMPARISON_GT"
      threshold_value    = var.error_rate_threshold
      duration           = "0s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.label.service_name"]
      }
      denominator_aggregations {
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
    content   = "5xx が増えている。Error Reporting と Cloud Logging(severity>=ERROR)を見る。Runbook: ${local.runbook_url}#5xx-率"
  }

  depends_on = [google_project_service.monitoring]
}

# p95 の待ち時間: サービスごとに閾値が違う(ai は生成で数十秒かかる)
resource "google_monitoring_alert_policy" "latency_p95" {
  for_each = local.monitoring ? var.monitored_services : {}

  project      = var.project_id
  display_name = "[${var.project_id}] ${each.key} p95 latency > ${each.value.p95_latency_ms} ms"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "p95 request_latencies over 5 min"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"run.googleapis.com/request_latencies\" AND resource.labels.service_name = \"${each.key}\""
      comparison      = "COMPARISON_GT"
      threshold_value = each.value.p95_latency_ms
      duration        = "300s"
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_PERCENTILE_95"
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
    content   = "p95 の待ち時間が閾値を 5 分超えている。Runbook: ${local.runbook_url}#p95-の待ち時間"
  }

  depends_on = [google_project_service.monitoring]
}

# Claude の失敗率: 率と件数の両方を超えたら(呼び出しが少ない時の誤報を防ぐ)
resource "google_monitoring_alert_policy" "claude_failure_rate" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] Claude failure rate > ${var.claude_failure_rate_threshold * 100}%"
  combiner     = "AND"
  severity     = "ERROR"

  conditions {
    display_name = "outcome=error / all attempts (10 min window)"
    condition_threshold {
      filter             = "${local.claude_calls_filter} AND metric.labels.outcome = \"error\""
      denominator_filter = local.claude_calls_filter
      comparison         = "COMPARISON_GT"
      threshold_value    = var.claude_failure_rate_threshold
      duration           = "0s"
      aggregations {
        alignment_period     = "600s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.label.service_name"]
      }
      denominator_aggregations {
        alignment_period     = "600s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
        group_by_fields      = ["resource.label.service_name"]
      }
      trigger {
        count = 1
      }
    }
  }

  conditions {
    display_name = "outcome=error count (10 min window)"
    condition_threshold {
      filter          = "${local.claude_calls_filter} AND metric.labels.outcome = \"error\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.claude_failure_min_count - 1
      duration        = "0s"
      aggregations {
        alignment_period     = "600s"
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
    content   = "Claude の呼び出しの失敗が多い。error_kind で内訳を見る(429 = レート制限、529 / overloaded = Anthropic 側)。Runbook: ${local.runbook_url}#claude-の失敗率"
  }

  depends_on = [google_logging_metric.claude_calls]
}

resource "google_monitoring_alert_policy" "panics" {
  count = local.monitoring ? 1 : 0

  project      = var.project_id
  display_name = "[${var.project_id}] backend panic recovered"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "panic count > 0 (5 min window)"
    condition_threshold {
      filter          = "resource.type = \"cloud_run_revision\" AND metric.type = \"logging.googleapis.com/user/backend_panics\" AND ${local.service_filter}"
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
    content   = "panic を Recover した。スタックは Error Reporting で見る。Runbook: ${local.runbook_url}#panic"
  }

  depends_on = [google_logging_metric.panics]
}

# ---- ダッシュボード ----

resource "google_monitoring_dashboard" "backend" {
  count = local.monitoring ? 1 : 0

  project = var.project_id
  # service_filter は JSON の文字列の中に入れるので " をエスケープする
  dashboard_json = templatefile("${path.module}/dashboards/backend.json.tftpl", {
    project        = var.project_id
    service_filter = replace(local.service_filter, "\"", "\\\"")
    runbook_url    = local.runbook_url
  })

  depends_on = [google_logging_metric.claude_calls, google_logging_metric.panics]
}
