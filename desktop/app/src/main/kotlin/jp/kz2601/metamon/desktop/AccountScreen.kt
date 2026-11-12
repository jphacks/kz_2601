package jp.kz2601.metamon.desktop

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.URI

/** 公式サイトの URL(Android の `LegalDocument`・`CONTACT_URL`、iOS の `LegalDocument` と同じ)。ここだけで持つ。 */
object SiteUrls {
    const val SITE = "https://tailor-it.app"
    const val TERMS = "$SITE/terms"
    const val PRIVACY = "$SITE/privacy"
    const val CONTACT = "$SITE/contact"
}

/** アカウントの画面の下のリンク(#2505)。並びはこの順。[url] が null の物はアプリの中で開く。 */
enum class AccountLink(val titleKey: String, val url: String?) {
    TERMS("legal_terms", SiteUrls.TERMS),
    PRIVACY("legal_privacy_policy", SiteUrls.PRIVACY),
    LICENSES("settings_licenses", null),
    CONTACT("settings_contact", SiteUrls.CONTACT),
    ;

    val title: String get() = Strings[titleKey]

    /** 押した結果。 */
    enum class Opened { BROWSER, IN_APP, FAILED }

    /** 開く。URL は既定のブラウザ(開けなければ [Opened.FAILED])、ライセンスは [onLicenses]。[browse] は UI のスレッドで呼ばない。 */
    fun open(onLicenses: () -> Unit, browse: (URI) -> Boolean = DesktopLicenses::browse): Opened {
        val u = url ?: return Opened.IN_APP.also { onLicenses() }
        return if (browse(URI(u))) Opened.BROWSER else Opened.FAILED
    }
}

/**
 * アカウントの画面。サインイン・サインアウト(#2155)が入るまでは、下のリンクだけ。
 * コマンドパレットの「アカウント」から開く。
 */
@Composable
fun AccountDialog(onLicenses: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(Strings["account_menu"], modifier = Modifier.semantics { heading() }) },
        text = { AccountLinksSection(onLicenses) },
        confirmButton = { TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) } },
    )
}

/** 規約・プライバシーポリシー・ライセンス・問い合わせ。ブラウザで開けなければ URL を添えて知らせる(選んで写せる)。 */
@Composable
fun AccountLinksSection(onLicenses: () -> Unit, modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    var failed by remember { mutableStateOf<String?>(null) }
    Column(modifier) {
        AccountLink.entries.forEach { link ->
            TextButton(onClick = {
                failed = null
                if (link.url == null) {
                    link.open(onLicenses)
                } else {
                    scope.launch {
                        // browse は OS の起動を待つことがあるので UI のスレッドで呼ばない
                        val r = withContext(Dispatchers.IO) { link.open(onLicenses) }
                        if (r == AccountLink.Opened.FAILED) failed = link.url
                    }
                }
            }) { Text(link.title) }
        }
        failed?.let { url ->
            SelectionContainer {
                Text(LicensesText.browseFailed(url), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}
