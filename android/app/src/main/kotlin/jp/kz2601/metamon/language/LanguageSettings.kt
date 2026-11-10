package jp.kz2601.metamon.language

import android.content.Context
import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.MainActivity
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.voice.ClarifySection
import jp.kz2601.metamon.voice.ClarifySettings

/**
 * 表示言語の選択(iOS の設定の「言語」・#843)。設定の画面(#898)ができたらそこに置く。
 * 新しく作るアプリ・相談・音声認識の言語もこれに合わせる(作成済みのアプリの文言は変わらない)。
 */
@Composable
fun LanguageSection(selected: AppLanguage, onSelect: (AppLanguage) -> Unit, modifier: Modifier = Modifier) {
    Column(modifier.selectableGroup().testTag("settings.language")) {
        Text(
            stringResource(R.string.language_title),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        AppLanguage.entries.forEach { language ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .selectable(selected = language == selected, role = Role.RadioButton) { onSelect(language) }
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                RadioButton(selected = language == selected, onClick = null)
                Text(
                    language.nativeName ?: stringResource(R.string.language_system),
                    modifier = Modifier.padding(start = 16.dp),
                )
            }
        }
        Text(
            stringResource(R.string.language_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
    }
}

/**
 * 言語だけの設定の画面。端末の設定 > アプリ > Tailor It の「アプリの設定」(`APPLICATION_PREFERENCES`)からも開ける。
 */
class LanguageSettingsActivity : ComponentActivity() {
    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocales.wrap(newBase))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            MetamonRenderTheme {
                var selected by remember { mutableStateOf(AppLocales.setting(this)) }
                Scaffold { padding ->
                    Column(Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState())) {
                        TextButton(onClick = ::finish, modifier = Modifier.padding(4.dp)) {
                            Text(stringResource(R.string.language_close))
                        }
                        LanguageSection(selected, onSelect = { language ->
                            if (language == selected) return@LanguageSection
                            selected = language
                            if (AppLocales.set(this@LanguageSettingsActivity, language)) restartApp()
                        })
                        // 相談の ON / OFF(#893)。設定の画面(#898)ができたらそこへ移す
                        ClarifySection(remember { ClarifySettings(this@LanguageSettingsActivity) })
                    }
                }
            }
        }
    }

    /** Android 12 / 12L: 開いている画面を新しい言語で作り直す(13+ はシステムがやる)。 */
    private fun restartApp() {
        startActivity(
            Intent(this, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK),
        )
        finish()
    }
}
