package jp.kz2601.metamon.renderer.components

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import androidx.browser.customtabs.CustomTabsIntent
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import java.net.URI

/** URL として読めるか(iOS の `URL(string:)`)。空・空白入りは読めない。 */
private fun parseUrl(s: String?): URI? {
    if (s.isNullOrEmpty()) return null
    return runCatching { URI(s) }.getOrNull()
}

/** 許可されたスキーム([ExternalUrlPolicy])で、URL として読めるものだけ。 */
private fun openableUri(s: String?): URI? = ExternalUrlPolicy.openable(s)?.let(::parseUrl)

/** link が出す文字(text が無ければ URL)と、押せるか(適合テストの `opens`)。 */
fun Component.linkText(): String = text ?: parseUrl(url)?.toString().orEmpty()

val Component.linkOpens: Boolean get() = parseUrl(url) != null

/** https は Custom Tabs、それ以外(tel・mailto・line …)は対応アプリで開く。開けるアプリが無ければ何もしない。 */
fun openExternalUrl(context: Context, uri: URI) {
    val u = android.net.Uri.parse(uri.toString())
    runCatching {
        if (uri.scheme.equals("https", ignoreCase = true)) {
            CustomTabsIntent.Builder().setShowTitle(true).build().launchUrl(context, u)
        } else {
            context.startActivity(Intent(Intent.ACTION_VIEW, u).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        }
    }.onFailure { if (it !is ActivityNotFoundException) throw it }
}

/** `link`: URL を開く文字のリンク。許されない URL は押せない控えめな文字。 */
@Composable
fun LinkLeaf(c: Component, modifier: Modifier = Modifier) {
    val target = openableUri(c.url)
    val style = MaterialTheme.typography.bodyLarge.copy(fontWeight = FontWeight.Medium)
    if (target == null) {
        Text(c.text.orEmpty(), modifier.fillMaxWidth(), style = style, color = MaterialTheme.colorScheme.onSurfaceVariant)
        return
    }
    val context = LocalContext.current
    val color = RenderTheme.colors.brandText
    Row(
        modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 44.dp)
            .clickable(role = Role.Button, onClickLabel = stringResource(R.string.lists_open)) { openExternalUrl(context, target) },
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SymbolIcon("arrow.up.right.square", 18.dp, color)
        Text(c.linkText(), style = style, color = color)
    }
}
