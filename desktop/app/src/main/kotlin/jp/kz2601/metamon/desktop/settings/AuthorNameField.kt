package jp.kz2601.metamon.desktop.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.social.AuthorProfile

/** 設定の「投稿に表示する名前」(Android の `settings_author_name_*` と同じ文言)。 */
object AuthorNameText {
    val HEADER get() = Strings["settings_author_name_header"]
    val PLACEHOLDER get() = Strings["settings_author_name_placeholder"]
    val FOOTER get() = Strings["settings_author_name_footer"]
}

/**
 * 共有の投稿に出す表示名の欄。打つたびに [profile] へ保存し(前後の空白は落とし・上限の長さで切る)、欄を離れたら保存した形に直す。
 * 空なら送らない(backend が「匿名」を付ける)。OS のユーザー名・PC の名前は入れない。
 */
@Composable
fun AuthorNameField(profile: AuthorProfile, modifier: Modifier = Modifier) {
    var text by remember(profile) { mutableStateOf(profile.name.orEmpty()) }
    Column(modifier) {
        OutlinedTextField(
            value = text,
            onValueChange = {
                text = it.take(AuthorProfile.MAX_LENGTH)
                profile.name = text
            },
            label = { Text(AuthorNameText.HEADER) },
            placeholder = { Text(AuthorNameText.PLACEHOLDER) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().onFocusChanged { if (!it.isFocused) text = profile.name.orEmpty() },
        )
        Text(AuthorNameText.FOOTER, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
