package jp.kz2601.metamon.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.social.AuthorProfile

/**
 * 投稿に表示する名前(#875・iOS の `AuthorNameSettingsSection`)。空なら匿名・[AuthorProfile.MAX_LENGTH] 文字まで。
 * 書き換えたら、初めての投稿で聞く確認も済みにする(iOS と同じ)。
 */
@Composable
fun AuthorNameSection(profile: AuthorProfile, modifier: Modifier = Modifier) {
    var text by remember(profile) { mutableStateOf(profile.name.orEmpty()) }
    Column(modifier.fillMaxWidth()) {
        Text(
            stringResource(R.string.settings_author_name_header),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        OutlinedTextField(
            value = text,
            onValueChange = { value ->
                text = value.take(AuthorProfile.MAX_LENGTH)
                profile.name = text
                profile.asked = true
            },
            singleLine = true,
            placeholder = { Text(stringResource(R.string.settings_author_name_placeholder)) },
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).testTag("settings.authorName"),
        )
        Text(
            stringResource(R.string.settings_author_name_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
    }
}
