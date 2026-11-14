package jp.kz2601.metamon.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.RecordRetention
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.runtime.AppRuntime

/** 設定の「記録の自動削除」の 1 行(iOS の `RetentionSettingsSection` の行)。 */
data class RetentionSettingsRow(
    val entity: String,
    val name: String,
    val days: Int,
    val isShared: Boolean,
    /** 確認済みで今消している。false なら「確認して自動で消す」を出す(「あとで」にした分もここから確認し直す) */
    val active: Boolean,
) {
    companion object {
        /** 保存期間のある entity だけ(DSL の順)。 */
        fun of(entities: List<Entity>, consented: (entity: String) -> Int?): List<RetentionSettingsRow> =
            entities.mapNotNull { e ->
                val days = e.retentionDays ?: return@mapNotNull null
                val active = RecordRetention.activeDays(e, consented(e.id)) != null
                RetentionSettingsRow(e.id, e.name, days, e.isShared, active)
            }
    }
}

/**
 * 設定の「記録の自動削除」(#482・iOS の `RetentionSettingsSection`)。今のアプリの期間と、確認・解除の入口。
 * 期間のある entity が無い・保存期間を扱わない(モック)時は出さない。
 * @param onStop 「自動で消すのをやめる」の指示文を送る(声で言うのと同じ既存の修正)
 */
@Composable
fun RetentionSettingsSection(runtime: AppRuntime, onStop: (String) -> Unit) {
    val retention = runtime.retention ?: return
    val document = runtime.state.collectAsStateWithLifecycle().value.document
    // 確認の保存は観測できないので、押したら数え直す
    var revision by remember { mutableIntStateOf(0) }
    val rows = remember(document.appId, document.version, revision) {
        RetentionSettingsRow.of(runtime.entityModels) { retention.consentedDays(document.appId, it) }
    }
    if (rows.isEmpty()) return
    val context = LocalContext.current
    val resources = LocalResources.current
    val english = AppLocales.languageCode(context) == "en"

    Text(
        stringResource(R.string.retention_settings_header),
        style = MaterialTheme.typography.titleSmall,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
    for (row in rows) {
        Column(Modifier.fillMaxWidth().testTag("settings.retention.${row.entity}")) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
                Text(row.name, modifier = Modifier.weight(1f))
                Text(
                    stringResource(R.string.retention_settings_period, RecordRetention.periodLabel(row.days, english)),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (!row.active) {
                TextButton(
                    onClick = {
                        val entity = runtime.entityModel(row.entity) ?: return@TextButton
                        retention.confirm(document.appId, entity)
                        runtime.sweepRetention(row.entity)
                        revision++
                    },
                    modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.retention.${row.entity}.confirm"),
                ) { Text(stringResource(R.string.retention_settings_confirm)) }
            }
            TextButton(
                onClick = {
                    retention.dismiss()
                    // 声で言うのと同じ指示文(iOS の `removeRetention(entityName:)`)
                    onStop(resources.getString(R.string.retention_stop_instruction_entity, row.name))
                },
                modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.retention.${row.entity}.stop"),
            ) { Text(stringResource(R.string.retention_stop), color = MaterialTheme.colorScheme.error) }
        }
    }
    val footer = stringResource(R.string.retention_settings_footer)
    val shared = stringResource(R.string.retention_settings_footer_shared)
    Text(
        if (rows.any { it.isShared }) footer + shared else footer,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
    HorizontalDivider(Modifier.padding(vertical = 8.dp))
}
