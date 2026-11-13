package jp.kz2601.metamon.health

import android.content.Context
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.consent.LegalLinks
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme

/**
 * Health Connect の「権限の理由」の画面(#870)。Health Connect が権限を聞く前・OS の設定の
 * 「アプリがデータを使う理由」から開かれる(`androidx.health.ACTION_SHOW_PERMISSIONS_RATIONALE`・
 * 14+ は `VIEW_PERMISSION_USAGE` の activity-alias)。
 * 中身は何に使うかの説明とプライバシーポリシーへの案内だけ(ポリシー本文はここでは持たない)。
 */
class HealthPermissionsRationaleActivity : ComponentActivity() {
    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocales.wrap(newBase))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            MetamonRenderTheme {
                Scaffold { padding ->
                    Column(
                        Modifier.fillMaxSize().padding(padding).verticalScroll(rememberScrollState()).padding(horizontal = 16.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        TextButton(onClick = ::finish, modifier = Modifier.padding(top = 4.dp)) {
                            Text(stringResource(R.string.health_rationale_close))
                        }
                        Text(stringResource(R.string.health_rationale_title), style = MaterialTheme.typography.titleLarge)
                        Text(stringResource(R.string.health_rationale_read), style = MaterialTheme.typography.bodyMedium)
                        Text(stringResource(R.string.health_rationale_write), style = MaterialTheme.typography.bodyMedium)
                        Text(
                            stringResource(R.string.health_rationale_note),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        LegalLinks()
                    }
                }
            }
        }
    }
}
