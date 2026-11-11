package jp.kz2601.metamon.settings

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.UsageMeter
import kotlinx.coroutines.CancellationException

/** 設定の「使っている量」の中身(iOS の `UsageSettingsSection`)。行が無ければ節ごと出さない。 */
data class UsageSectionModel(val rows: List<Row>, val pro: Boolean) {
    enum class Resource(val tag: String, @StringRes val title: Int) {
        APPS("apps", R.string.settings_usage_apps),
        RECORDS_PER_APP("recordsPerApp", R.string.settings_usage_records),
        PHOTOS_PER_APP("photosPerApp", R.string.settings_usage_photos),
    }

    data class Row(val resource: Resource, val meter: UsageMeter) {
        /** 80% 以上は目立たせる */
        val near: Boolean get() = meter.isNear

        /** 「3 / 100」。数えない時は null(「3(上限なし)」) */
        val amount: String? get() = meter.limit?.let { "${meter.used} / $it" }
    }

    companion object {
        /**
         * 作ったアプリの数(`GET /v1/me/usage`)と、今のアプリの記録・写真(`GET /v1/apps/{appId}/usage`・作成者の時だけ)。
         * Pro は「今日の残り」で分かった Pro・自分のプラン・今のアプリのプランのどれか。
         */
        fun of(me: MyUsage?, app: AppUsage?, isPro: Boolean): UsageSectionModel {
            val mine = app?.takeIf { it.isMine }
            val rows = buildList {
                me?.apps?.let { add(Row(Resource.APPS, it)) }
                mine?.records?.let { add(Row(Resource.RECORDS_PER_APP, it)) }
                mine?.photos?.let { add(Row(Resource.PHOTOS_PER_APP, it)) }
            }
            return UsageSectionModel(rows, pro = isPro || me?.isPro == true || mine?.plan == "pro")
        }
    }
}

/**
 * 設定の「使っている量」(#317・iOS の `UsageSettingsSection`)。作ったアプリの数と、今のアプリの記録・写真(作成者の時だけ)。
 * 自分の分([me])はシートを開いた時に音声の画面が読み直した値(5 分キャッシュ・iOS の `refreshPlanUsage` と同じく 1 か所で読む)。
 * 今のアプリの分だけここで読む。モック・古いサーバー(404)・読めない時は出さない。Android はアプリ内で Pro にできないので、
 * 無料の時の下書きは iOS の課金の無い版と同じ「使わないものを消すと、空きが増えます」。
 */
@Composable
fun UsageSection(container: AppContainer, me: MyUsage?, isPro: Boolean) {
    val source = container.usageSource
    val state = container.runtime.state.collectAsStateWithLifecycle().value
    val appId = state.document.appId.takeUnless { state.dsl.isVoiceOnly }
    val app by produceState<AppUsage?>(null, source, appId) { value = loadAppUsage(source, appId) }
    val model = UsageSectionModel.of(me, app, isPro)
    if (model.rows.isEmpty()) return

    Text(
        stringResource(R.string.settings_usage_header),
        style = MaterialTheme.typography.titleSmall,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
    model.rows.forEach { row ->
        val title = stringResource(row.resource.title)
        val amount = row.amount ?: stringResource(R.string.settings_usage_no_limit, row.meter.used)
        val spoken = row.meter.limit?.let { stringResource(R.string.settings_usage_a11y_amount, it, row.meter.used) }
            ?: stringResource(R.string.settings_usage_a11y_no_limit, row.meter.used)
        Row(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 8.dp)
                .clearAndSetSemantics {
                    contentDescription = title
                    stateDescription = spoken
                }
                .testTag("settings.usage.${row.resource.tag}"),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(title, modifier = Modifier.weight(1f))
            Text(amount, color = if (row.near) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
    Text(
        stringResource(if (model.pro) R.string.settings_usage_footer_pro else R.string.settings_usage_footer_free),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

/** 今のアプリの記録・写真(`GET /v1/apps/{appId}/usage`)。自分の分(`/v1/me/usage`)はここでは読まない。 */
internal suspend fun loadAppUsage(source: PlanUsageSource?, appId: String?): AppUsage? =
    if (source == null || appId == null) null else readOrNull { source.appUsage(appId) }

private suspend fun <T> readOrNull(read: suspend () -> T?): T? = try {
    read()
} catch (e: CancellationException) {
    throw e
} catch (_: Throwable) {
    null
}
