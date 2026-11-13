package jp.kz2601.metamon.desktop

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
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

/** アカウントの状態(#2374)。トークンは持たない(表示に要る物だけ)。 */
sealed interface AccountState {
    data object SignedOut : AccountState

    /** [anonymous] は匿名で始めた(Google にまだ引き継いでいない)。 */
    data class SignedIn(val displayName: String?, val email: String?, val anonymous: Boolean) : AccountState
}

/** アカウントの画面のボタン。文言は Android と同じキー。 */
enum class AccountButton(val titleKey: String) {
    START_ANONYMOUS("auth_anonymous"),
    SIGN_IN_GOOGLE("auth_google"),
    LINK_GOOGLE("account_link_google"),
    SIGN_OUT("account_sign_out"),
    ;

    val title: String get() = Strings[titleKey]
}

/** ボタンの中身(サインイン #2151・サインアウト #2375 が差し込む)。null のボタンは押せない。 */
data class AccountActions(
    val startAnonymous: (() -> Unit)? = null,
    val signInGoogle: (() -> Unit)? = null,
    val linkGoogle: (() -> Unit)? = null,
    val signOut: (() -> Unit)? = null,
) {
    fun of(button: AccountButton): (() -> Unit)? = when (button) {
        AccountButton.START_ANONYMOUS -> startAnonymous
        AccountButton.SIGN_IN_GOOGLE -> signInGoogle
        AccountButton.LINK_GOOGLE -> linkGoogle
        AccountButton.SIGN_OUT -> signOut
    }
}

/** 状態 → 出す物(描画に頼らない。テストはこれを見る)。 */
object AccountScreenModel {
    fun buttons(state: AccountState): List<AccountButton> = when (state) {
        AccountState.SignedOut -> listOf(AccountButton.START_ANONYMOUS, AccountButton.SIGN_IN_GOOGLE)
        is AccountState.SignedIn ->
            if (state.anonymous) listOf(AccountButton.LINK_GOOGLE, AccountButton.SIGN_OUT) else listOf(AccountButton.SIGN_OUT)
    }

    /** 状態の行(上から)。匿名は名前・メールを出さない。 */
    fun statusLines(state: AccountState): List<String> = when (state) {
        AccountState.SignedOut -> listOf(Strings["desktop_account_signed_out"])
        is AccountState.SignedIn ->
            if (state.anonymous) {
                listOf(Strings["account_link_status_anonymous"])
            } else {
                listOfNotNull(state.displayName, state.email).map { it.trim() }.filter { it.isNotEmpty() }.distinct()
                    .ifEmpty { listOf(Strings["desktop_account_signed_in"]) }
            }
    }
}

/** 今のアカウントの状態とボタンの中身。全部のウィンドウで 1 つ(サインイン・サインアウトが書き換える)。 */
object DesktopAccount {
    var state: AccountState by mutableStateOf(AccountState.SignedOut)
    var actions: AccountActions by mutableStateOf(AccountActions())
}

/**
 * アカウントの画面(#2374)。上に状態とボタン、下にリンク(#2505)。
 * コマンドパレット・トレイの「アカウント」から開く。
 */
@Composable
fun AccountDialog(
    onLicenses: () -> Unit,
    state: AccountState = DesktopAccount.state,
    actions: AccountActions = DesktopAccount.actions,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(Strings["account_menu"], modifier = Modifier.semantics { heading() }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                AccountStatusSection(state, actions)
                AccountLinksSection(onLicenses)
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) } },
    )
}

/** 状態の行(読み上げは 1 つにまとめる)とボタン。 */
@Composable
fun AccountStatusSection(state: AccountState, actions: AccountActions, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        val lines = AccountScreenModel.statusLines(state)
        Column(Modifier.semantics(mergeDescendants = true) { contentDescription = A11y.accountStatus(lines) }) {
            lines.forEachIndexed { i, line ->
                Text(line, style = if (i == 0) MaterialTheme.typography.titleSmall else MaterialTheme.typography.bodySmall)
            }
        }
        AccountScreenModel.buttons(state).forEach { button ->
            val run = actions.of(button)
            if (button == AccountButton.SIGN_OUT) {
                OutlinedButton(onClick = { run?.invoke() }, enabled = run != null) { Text(button.title) }
            } else {
                Button(onClick = { run?.invoke() }, enabled = run != null) { Text(button.title) }
            }
        }
    }
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
