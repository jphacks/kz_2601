package jp.kz2601.metamon.desktop

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.update.ServiceNotice

/**
 * アプリ内のお知らせの帯の中身(iOS の `ServiceNoticeBanner`・Android の `ServiceNoticeBanner` と同じ形)。
 * 出すかどうか(閉じた id・iOS の版の範囲)は [jp.kz2601.metamon.desktop.net.DesktopNotice.toShow] が決め、ここは出す物だけを作る。
 */
data class NoticeBannerModel(
    val text: String,
    /** 色だけでなく文字の印でも分ける(WARNING は「重要」) */
    val tag: String,
    val warning: Boolean,
    /** 読み上げる 1 つの文。操作(詳しく・閉じる)は別のボタン */
    val description: String,
    /** 「詳しく」で開く https の URL。無ければ出さない */
    val link: String?,
) {
    companion object {
        fun of(notice: ServiceNotice, lang: Lang = Strings.lang): NoticeBannerModel {
            val text = notice.text(if (lang == Lang.EN) "en" else "ja")
            val warning = notice.severity == ServiceNotice.Severity.WARNING
            return NoticeBannerModel(
                text = text,
                tag = Strings.table.get(lang, if (warning) "desktop_notice_tag_warning" else "desktop_notice_tag_info"),
                warning = warning,
                description = Strings.table.format(lang, if (warning) "notice_a11y_warning" else "notice_a11y_info", text),
                link = notice.link,
            )
        }
    }
}

/**
 * すべてのウィンドウの上に出す 1 行の帯。[notice] が null なら何も出さない。
 * 「詳しく」は [onOpenLink] に https の URL を渡す(ブラウザで開くのは呼び出し側)。閉じたら [onDismiss](同じ id は次の起動でも出さない)。
 */
@Composable
fun NoticeBanner(notice: ServiceNotice?, onOpenLink: (String) -> Unit, onDismiss: (String) -> Unit) {
    if (notice == null) return
    val model = remember(notice, Strings.lang) { NoticeBannerModel.of(notice) }
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth()) {
        Row(
            Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // 1 つの知らせとして読む(印・文)。ボタンは別
            Row(
                Modifier.weight(1f).padding(vertical = 6.dp)
                    .semantics(mergeDescendants = true) {
                        contentDescription = model.description
                        liveRegion = LiveRegionMode.Polite
                    },
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text(
                    model.tag,
                    style = MaterialTheme.typography.labelMedium,
                    fontWeight = FontWeight.Bold,
                    color = if (model.warning) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(model.text, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            }
            model.link?.let { url ->
                TextButton(onClick = { onOpenLink(url) }) { Text(Strings["notice_learn_more"], fontWeight = FontWeight.SemiBold) }
            }
            TextButton(onClick = { onDismiss(notice.id) }) { Text(Strings["notice_dismiss"]) }
        }
    }
}
