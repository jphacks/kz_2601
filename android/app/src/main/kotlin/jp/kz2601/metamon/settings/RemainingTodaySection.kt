package jp.kz2601.metamon.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.voice.RemainingTodayDisplay
import jp.kz2601.metamon.voice.resetText
import java.time.Instant

/** 設定の「今日の残り」の行の中身(iOS の `RemainingTodaySettingsSection`)。 */
data class RemainingTodayRow(
    /** 「24 / 60」(上限が分からなければ数だけ)。古い・まだ分からない時は null(「AI を使うと表示されます」) */
    val amount: String?,
    /** 数が分かっていて Pro の時だけ印を出す */
    val pro: Boolean,
    /** 少ない([RemainingTodayDisplay.LOW_THRESHOLD] 以下)。目立たせる */
    val low: Boolean,
    /** 戻る時刻(分かる時だけ行を出す) */
    val resetAt: Instant?,
) {
    companion object {
        fun of(value: RemainingToday?, now: Instant, isPro: Boolean): RemainingTodayRow {
            val current = value?.takeUnless { it.isStale(now) } ?: return RemainingTodayRow(null, pro = false, low = false, resetAt = null)
            val amount = current.limit?.let { "${current.remaining} / $it" } ?: "${current.remaining}"
            return RemainingTodayRow(
                amount = amount,
                pro = isPro,
                low = current.remaining <= RemainingTodayDisplay.LOW_THRESHOLD,
                resetAt = current.resetAt,
            )
        }
    }
}

/**
 * 設定の「今日の残り」(#630・iOS の `RemainingTodaySettingsSection`)。最後に分かった残り・Pro・戻る時刻。
 * 値は音声の画面と同じ `GET /v1/me/usage`(シートを開いた時に読み直す)。AI を使うまで・モックでは分からない。
 */
@Composable
fun RemainingTodaySection(value: RemainingToday?, isPro: Boolean) {
    val row = RemainingTodayRow.of(value, Instant.now(), isPro)
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp).semantics(mergeDescendants = true) {}.testTag("settings.remainingToday"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Text(stringResource(R.string.voice_remaining_settings_title), modifier = Modifier.weight(1f))
        if (row.pro) Text("Pro", fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary)
        Text(
            row.amount ?: stringResource(R.string.voice_remaining_settings_unknown),
            color = if (row.low) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
    row.resetAt?.let { at ->
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).testTag("settings.remainingToday.reset")) {
            Text(stringResource(R.string.voice_remaining_settings_reset), modifier = Modifier.weight(1f))
            Text(resetText(at), color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
    Text(
        stringResource(
            R.string.voice_remaining_hint,
            RemainingToday.Weight.CREATE,
            RemainingToday.Weight.EDIT,
            RemainingToday.Weight.CHAT,
            resetText(row.resetAt),
        ),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}
