package jp.kz2601.metamon.desktop.store

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.RecordRetention

/**
 * 保存期間のある entity の確認(#482・Android の `RetentionPromptHost`・iOS の `RetentionPromptHost`)と、設定の見直しの一覧。
 * 「この記録は N 日で自動で消えます」を初めて開いた時に聞き、確認するまで消さない([RetentionRunner.confirm] が書く印を、消す処理が見る)。
 */
object RetentionPrompts {
    /** 確認ダイアログの中身。[items] は確認が要る entity(DSL の順)。 */
    data class Prompt(val appId: String, val items: List<Entity>) {
        val key: String get() = items.joinToString(",") { key(appId, it) }

        /** 「「支出」は 30日 … たった記録を自動で消します。」(共有の entity があれば、自分が追加した物だけの注意を足す) */
        fun message(lang: Lang): String {
            val t = Strings.table
            val parts = items.joinToString(t.get(lang, "retention_message_separator")) {
                t.format(lang, "retention_message_item", it.name, period(it.retentionDays ?: 0, lang))
            }
            val shared = if (items.any { it.isShared }) t.get(lang, "retention_settings_footer_shared") else ""
            return t.format(lang, "retention_message", parts) + shared
        }
    }

    /** 設定の見直しの 1 行。 */
    data class Row(val appName: String, val entityId: String, val entityName: String, val days: Int, val confirmed: Boolean) {
        fun period(lang: Lang): String = Strings.table.format(lang, "retention_settings_period", period(days, lang))

        /** 読み上げる 1 行: アプリ・entity・日数・確認済みか。 */
        fun description(lang: Lang): String = listOf(
            appName,
            entityName,
            period(lang),
            Strings.table.get(lang, if (confirmed) "desktop_retention_confirmed" else "desktop_retention_unconfirmed"),
        ).joinToString(Strings.table.get(lang, "desktop_list_separator"))
    }

    private val deferred = mutableSetOf<String>()

    private fun key(appId: String, e: Entity) = "$appId/${e.id}=${e.retentionDays ?: 0}"

    fun period(days: Int, lang: Lang): String = RecordRetention.periodLabel(days, lang == Lang.EN)

    /** いま聞くこと。確認が要る entity のうち「あとで」にしていない物。無ければ null。 */
    fun pending(appId: String, runner: RetentionRunner, models: List<Entity>): Prompt? {
        val items = runner.pending(appId, models).filter { synchronized(deferred) { key(appId, it) !in deferred } }
        return if (items.isEmpty()) null else Prompt(appId, items)
    }

    /** 「自動で消す」: 確認した期間を端末に書く。これ以降、期限を過ぎた記録を消してよい。 */
    fun confirm(runner: RetentionRunner, prompt: Prompt) {
        prompt.items.forEach { runner.confirm(prompt.appId, it) }
    }

    /** 設定の一覧から 1 つだけ確認する。 */
    fun confirm(runner: RetentionRunner, appId: String, entity: Entity) = runner.confirm(appId, entity)

    /** 「あとで」: このプロセスの間は聞き直さない(消さない)。 */
    fun defer(prompt: Prompt) {
        synchronized(deferred) { prompt.items.forEach { deferred += key(prompt.appId, it) } }
    }

    /** 「自動で消える記録」の一覧(保存期間のある entity だけ・DSL の順)。確認済み = 今の期間以下で確認した。 */
    fun rows(appId: String, appName: String, runner: RetentionRunner, models: List<Entity>): List<Row> {
        val pending = runner.pending(appId, models).map { it.id }.toSet()
        return models.filter { it.retentionDays != null }.map { Row(appName, it.id, it.name, it.retentionDays ?: 0, it.id !in pending) }
    }
}

/** 初めて開いた時の確認。「自動で消す」を押すまで消さない。 */
@Composable
fun RetentionPromptDialog(prompt: RetentionPrompts.Prompt, lang: Lang = Strings.lang, onConfirm: () -> Unit, onLater: () -> Unit) {
    AlertDialog(
        onDismissRequest = onLater,
        title = { Text(Strings["retention_title"], modifier = Modifier.semantics { heading() }) },
        text = { Text(prompt.message(lang)) },
        confirmButton = { TextButton(onClick = onConfirm) { Text(Strings["retention_confirm"], color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = onLater) { Text(Strings["retention_later"]) } },
    )
}

/** 設定の「自動で消える記録」(アプリ・entity・日数・確認済みか)。未確認の行には「確認して自動で消す」。 */
@Composable
fun RetentionReviewDialog(rows: List<RetentionPrompts.Row>, lang: Lang = Strings.lang, onConfirm: (RetentionPrompts.Row) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.widthIn(max = 560.dp),
        title = { Text(Strings["retention_settings_header"], modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(Strings["retention_settings_footer"], style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 360.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    items(rows, key = { "${it.appName}/${it.entityId}" }) { r ->
                        Row(
                            Modifier.fillMaxWidth().padding(vertical = 4.dp),
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            Column(Modifier.weight(1f).semantics(mergeDescendants = true) { contentDescription = r.description(lang) }) {
                                Text(r.entityName)
                                Text(
                                    listOf(r.appName, r.period(lang)).joinToString(Strings["desktop_list_separator"]),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (r.confirmed) {
                                Text(Strings["desktop_retention_confirmed"], style = MaterialTheme.typography.labelMedium)
                            } else {
                                TextButton(onClick = { onConfirm(r) }) { Text(Strings["retention_settings_confirm"]) }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) } },
    )
}
