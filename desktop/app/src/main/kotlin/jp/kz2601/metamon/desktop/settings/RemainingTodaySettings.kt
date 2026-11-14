package jp.kz2601.metamon.desktop.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.services.RemainingTodayStore
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** 設定の「今日の残り」の行の中身(#3361・Android の `RemainingTodayRow`・iOS の `RemainingTodaySettingsSection`)。 */
data class RemainingTodayRow(
    /** 「24 / 60」(上限が分からなければ数だけ)。古い・まだ分からない時は null(「AI を使うと表示されます」) */
    val amount: String?,
    /** 数が分かっていて Pro の時だけ印を出す */
    val pro: Boolean,
    /** 少ない([LOW_THRESHOLD] 以下)。目立たせる */
    val low: Boolean,
    /** 戻る時刻(分かる時だけ行を出す) */
    val resetAt: Instant?,
) {
    companion object {
        /** 新しく作るのがあと 2 回分を切ったら少ない(Android の `RemainingTodayDisplay.LOW_THRESHOLD` と同じ) */
        const val LOW_THRESHOLD = RemainingToday.Weight.CREATE * 2

        fun of(value: RemainingToday?, now: Instant, isPro: Boolean): RemainingTodayRow {
            val current = value?.takeUnless { it.isStale(now) } ?: return RemainingTodayRow(null, pro = false, low = false, resetAt = null)
            val amount = current.limit?.let { "${current.remaining} / $it" } ?: "${current.remaining}"
            return RemainingTodayRow(amount, pro = isPro, low = current.remaining <= LOW_THRESHOLD, resetAt = current.resetAt)
        }
    }
}

/** 文言(Android の `voice_remaining_settings_*`・`voice_remaining_hint` をそのまま引く)。 */
object RemainingTodayText {
    /** 右の値。分からない時は「AI を使うと表示されます」 */
    fun amount(row: RemainingTodayRow, lang: Lang = Strings.lang): String = row.amount ?: Strings.table.get(lang, "voice_remaining_settings_unknown")

    /** 戻る時刻(上限のカードと同じ書き方)。分からなければ「明日」 */
    fun resetText(at: Instant?, lang: Lang = Strings.lang, zone: ZoneId = ZoneId.systemDefault()): String {
        at ?: return Strings.table.get(lang, "voice_tomorrow")
        return DateTimeFormatter.ofPattern(Strings.table.get(lang, "desktop_reset_time_pattern"), lang.locale).withZone(zone).format(at)
    }

    /** 下の説明(新規 3・修正 2・相談 1 と戻る時刻) */
    fun hint(row: RemainingTodayRow, lang: Lang = Strings.lang, zone: ZoneId = ZoneId.systemDefault()): String = Strings.table.format(
        lang,
        "voice_remaining_hint",
        RemainingToday.Weight.CREATE,
        RemainingToday.Weight.EDIT,
        RemainingToday.Weight.CHAT,
        resetText(row.resetAt, lang, zone),
    )

}

/**
 * 設定の「今日の残り」(#3361)。最後に分かった残り・上限・戻る時刻を [store] から読む(ai の応答のヘッダー・`GET /v1/me/usage` が入れる)。
 * AI を使うまでは「AI を使うと表示されます」。
 */
@Composable
fun RemainingTodaySection(store: RemainingTodayStore, isPro: Boolean, modifier: Modifier = Modifier, now: () -> Instant = Instant::now) {
    val value by store.value.collectAsState()
    val row = RemainingTodayRow.of(value, now(), isPro)
    Column(modifier) {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {},
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            Text(Strings["voice_remaining_settings_title"], modifier = Modifier.weight(1f))
            if (row.pro) Text("Pro", fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary)
            Text(
                RemainingTodayText.amount(row),
                color = if (row.low) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        row.resetAt?.let { at ->
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).semantics(mergeDescendants = true) {}) {
                Text(Strings["voice_remaining_settings_reset"], modifier = Modifier.weight(1f))
                Text(RemainingTodayText.resetText(at), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        Text(
            RemainingTodayText.hint(row),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
    }
}
