package jp.kz2601.metamon.renderer.components.lists

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon

/** 「このデータは誰でも見られます(公開)」の控えめな表示(#253・iOS の `PublicDataNotice`)。DSL の `entity.scope` を読むだけ。 */
internal object PublicDataNotice {
    /** 文言。private・省略・未知の値は null(出さない)。 */
    @StringRes
    fun text(scope: String?): Int? = when (scope) {
        "public" -> R.string.lists_public_notice
        "shared" -> R.string.lists_shared_notice
        else -> null
    }

    fun icon(scope: String?): String = if (scope == "public") "globe" else "person.2"
}

/** 一覧の上に公開の表示を足す(entity が public / shared の時だけ)。iOS の `.publicDataNotice(entity:)`。 */
@Composable
internal fun PublicListNotice(scope: String?, modifier: Modifier, content: @Composable (Modifier) -> Unit) {
    val text = PublicDataNotice.text(scope)
    if (text == null) {
        content(modifier)
        return
    }
    Column(modifier, verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        val label = stringResource(text)
        Row(
            Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.xs).clearAndSetSemantics { contentDescription = label },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            SymbolIcon(PublicDataNotice.icon(scope), 14.dp, MaterialTheme.colorScheme.onSurfaceVariant)
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        content(Modifier)
    }
}
