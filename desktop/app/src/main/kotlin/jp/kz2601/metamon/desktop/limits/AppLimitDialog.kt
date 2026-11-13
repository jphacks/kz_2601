package jp.kz2601.metamon.desktop.limits

import androidx.compose.foundation.clickable
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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.util.Locale

/** アプリの数の上限のダイアログの文言。Android に同じ物があればそのキー、無ければ `desktop_applimit_*`。 */
object AppLimitText {
    val TITLE get() = Strings["voice_plan_limit_title"]
    val CANCEL get() = Strings["inputs_cancel"]
    val EMPTY get() = Strings["desktop_applimit_empty"]
    val CURRENT get() = Strings["desktop_applimit_row_current"]

    fun body(plan: AppReplacePlan): String = Strings.format("desktop_applimit_body", plan.held, plan.limit)

    /** 行の右に出す操作の名前(削除 / 抜ける) */
    fun actionLabel(row: AppReplacePlan.Row): String = when (row.role) {
        AppReplacePlan.Role.OWN -> Strings["runtime_confirm_delete"]
        AppReplacePlan.Role.JOINED -> Strings["members_leave_confirm"]
    }

    fun lastOpened(row: AppReplacePlan.Row, zone: ZoneId = ZoneId.systemDefault(), locale: Locale = Strings.locale): String =
        row.lastOpened?.let { Strings.format("desktop_applimit_last_opened", date(it, zone, locale)) } ?: Strings["desktop_applimit_never_opened"]

    private fun date(at: Instant, zone: ZoneId, locale: Locale): String =
        DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).withLocale(locale).format(at.atZone(zone))

    /** 読み上げる 1 行: アプリの名前・(使用中)・最後に開いた日・操作の種類。 */
    fun rowDescription(row: AppReplacePlan.Row, zone: ZoneId = ZoneId.systemDefault(), locale: Locale = Strings.locale): String =
        listOfNotNull(row.name, CURRENT.takeIf { row.isCurrent }, lastOpened(row, zone, locale), actionLabel(row))
            .joinToString(Strings["desktop_list_separator"])

    /** 選んだ後の確認の見出しと本文(削除はデータも消える旨・抜ける) */
    fun confirmTitle(row: AppReplacePlan.Row): String = when (row.role) {
        AppReplacePlan.Role.OWN -> Strings.format("desktop_applimit_confirm_delete", row.name)
        AppReplacePlan.Role.JOINED -> Strings.format("desktop_applimit_confirm_leave", row.name)
    }

    fun confirmBody(row: AppReplacePlan.Row): String = when (row.role) {
        AppReplacePlan.Role.OWN -> Strings["desktop_applimit_delete_message"]
        AppReplacePlan.Role.JOINED -> Strings["members_leave_message"]
    }
}

/**
 * 作る時に 402 `app_limit_reached` が返った時の、置き換えるアプリの選択。候補は [AppReplacePlan] の並び(長く開いていない順)。
 * 選ぶと確認(削除はデータも消える)を出し、確認したら [onConfirm] に削除 / 抜けるを渡す。呼び出し側は実行して、同じ依頼で作り直す。
 */
@Composable
fun AppLimitDialog(
    plan: AppReplacePlan,
    onConfirm: (AppReplacePlan.Action) -> Unit,
    onDismiss: () -> Unit,
) {
    var picked by remember(plan) { mutableStateOf<AppReplacePlan.Row?>(null) }
    val row = picked
    if (row != null) {
        AlertDialog(
            onDismissRequest = { picked = null },
            title = { Text(AppLimitText.confirmTitle(row), modifier = Modifier.semantics { heading() }) },
            text = { Text(AppLimitText.confirmBody(row)) },
            confirmButton = { TextButton(onClick = { onConfirm(AppReplacePlan.action(row)) }) { Text(AppLimitText.actionLabel(row)) } },
            dismissButton = { TextButton(onClick = { picked = null }) { Text(AppLimitText.CANCEL) } },
        )
        return
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.widthIn(max = 560.dp),
        title = { Text(AppLimitText.TITLE, modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(AppLimitText.body(plan), style = MaterialTheme.typography.bodySmall)
                if (plan.rows.isEmpty()) {
                    Text(AppLimitText.EMPTY, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 360.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    items(plan.rows, key = { it.id }) { r ->
                        Row(
                            Modifier.fillMaxWidth()
                                .clickable(role = Role.Button) { picked = r }
                                .semantics(mergeDescendants = true) { contentDescription = AppLimitText.rowDescription(r) }
                                .padding(horizontal = 10.dp, vertical = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            Column(Modifier.weight(1f)) {
                                Text(r.name)
                                Text(
                                    listOfNotNull(AppLimitText.CURRENT.takeIf { r.isCurrent }, AppLimitText.lastOpened(r))
                                        .joinToString(Strings["desktop_list_separator"]),
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            Text(AppLimitText.actionLabel(r), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(AppLimitText.CANCEL) } },
    )
}
