package jp.kz2601.metamon.auth

import jp.kz2601.metamon.R
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.mock.MockAuthService
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/** サインインの失敗の文言が画面の言語で出る(英語の端末で日本語を出さない・例外の文言を出さない)。 */
class AuthErrorTextTest {
    private val androidRoot: File = File(System.getProperty("user.dir")).absoluteFile.parentFile

    /** strings_auth.xml を読んで `Resources.getString` の代わりにする(日 = values・英 = values-en)。 */
    private fun text(ui: UiText?, english: Boolean): String? {
        val file = File(androidRoot, "app/src/main/res/${if (english) "values-en" else "values"}/strings_auth.xml")
        val nodes = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(file).getElementsByTagName("string")
        val texts = (0 until nodes.length).associate { i -> (nodes.item(i) as Element).let { it.getAttribute("name") to it.textContent } }
        return when (ui) {
            null -> null
            is UiText.Raw -> ui.text
            is UiText.Res -> texts.getValue(R.string::class.java.fields.first { it.getInt(null) == ui.id }.name)
        }
    }

    /** Firebase の失敗(Apple の Custom Tab・通信など)を真似る。 */
    private class FailingAuth(private val e: Exception) : AuthService by MockAuthService(signedIn = false) {
        override suspend fun signInAnonymously() = throw e

        override suspend fun signOut() = throw e
    }

    @Test
    fun signInFailureFollowsDeviceLanguage() = runTest {
        val session = AuthSession(FailingAuth(IllegalStateException("An internal error has occurred. [ 7: ]")))
        session.signInAnonymously()

        assertEquals("Couldn’t sign in. Please try again", text(session.errorMessage.value, english = true))
        assertEquals("サインインできませんでした。もう一度お試しください", text(session.errorMessage.value, english = false))
    }

    @Test
    fun googleTokenFailureFollowsDeviceLanguage() = runTest {
        val session = AuthSession(MockAuthService(signedIn = false))
        session.signInWithGoogle { throw IllegalStateException("network down") }

        assertEquals("Couldn’t sign in. Please try again", text(session.errorMessage.value, english = true))
    }

    @Test
    fun signOutFailureFollowsDeviceLanguage() = runTest {
        val session = AuthSession(FailingAuth(IllegalStateException("boom")))
        session.signOut()

        assertEquals("Couldn’t sign out", text(session.errorMessage.value, english = true))
        assertEquals("ログアウトできませんでした", text(session.errorMessage.value, english = false))
    }

    @Test
    fun exceptionTextIsNotShownExceptAppMadeUnavailable() {
        assertEquals(AuthSession.SIGN_IN_FAILED, AuthSession.failureText(IllegalStateException("network down")))
        assertEquals(AuthSession.SIGN_IN_FAILED, AuthSession.failureText(AuthError.RequiresRecentLogin()))
        assertEquals(AuthSession.SIGN_IN_FAILED, AuthSession.failureText(AuthError.Unavailable("")))
        // Unavailable はアプリが strings から作った文言(GoogleSignIn)なのでそのまま
        assertEquals(UiText.Raw("No account"), AuthSession.failureText(AuthError.Unavailable("No account")))
    }
}
