package jp.kz2601.metamon.account

import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.mock.MockAuthService
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/** アカウントの削除・ログアウトの失敗の本文が画面の言語で出る(例外の文言を出さない・iOS と同じ文言)。 */
class AccountFailureTextTest {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    /** strings_account.xml を読んで `Resources.getString` の代わりにする(日 = values・英 = values-en)。 */
    private fun text(ui: UiText, english: Boolean): String {
        val file = File(androidRoot, "app/src/main/res/${if (english) "values-en" else "values"}/strings_account.xml")
        val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("string")
        val texts = (0 until nodes.length).associate { i -> (nodes.item(i) as Element).let { it.getAttribute("name") to it.textContent } }
        return when (ui) {
            is UiText.Raw -> ui.text
            is UiText.Res -> texts.getValue(R.string::class.java.fields.first { it.getInt(null) == ui.id }.name)
            is UiText.Plural -> error("数で形が変わる文は使わない")
        }
    }

    /** 再認証を持たない実装(既定の実装・#1394)。 */
    private class DefaultsOnlyAuth : AuthService by MockAuthService(signedIn = false) {
        override suspend fun reauthenticateWithGoogle(googleIdToken: String) = super.reauthenticateWithGoogle(googleIdToken)
    }

    @Test
    fun firebaseExceptionTextIsNotShown() {
        val firebase = IllegalStateException("An internal error has occurred. [ 7: ]")
        val ui = AccountFailureText.of(firebase)

        assertEquals("Please wait a moment and try again", text(ui, english = true))
        assertEquals("少し待ってから、もう一度お試しください", text(ui, english = false))
        assertFalse(text(ui, english = true).contains("internal error"))
    }

    @Test
    fun defaultImplementationTextIsNotShown() = runTest {
        val e = runCatching { DefaultsOnlyAuth().reauthenticateWithGoogle("t") }.exceptionOrNull() as Exception

        val ui = AccountFailureText.of(e)
        assertEquals(AccountFailureText.TRY_AGAIN, ui)
        assertFalse(text(ui, english = true).contains("not supported"))
    }

    @Test
    fun requiresRecentLoginMatchesIos() {
        val ui = AccountFailureText.of(AuthError.RequiresRecentLogin())

        assertEquals("Sign in again, then delete", text(ui, english = true))
        assertEquals("もう一度サインインしてから削除してください", text(ui, english = false))
    }

    @Test
    fun appMadeUnavailableIsShownAsIs() {
        assertEquals(UiText.Raw("No account"), AccountFailureText.of(AuthError.Unavailable("No account")))
        assertEquals(AccountFailureText.TRY_AGAIN, AccountFailureText.of(AuthError.Unavailable("")))
    }
}
