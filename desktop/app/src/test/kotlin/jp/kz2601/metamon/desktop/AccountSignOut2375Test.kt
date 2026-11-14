package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.auth.InMemoryTokenStore
import jp.kz2601.metamon.desktop.auth.StoredCredential
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.store.SignOutChoice
import jp.kz2601.metamon.desktop.store.SignOutCleanup
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** サインアウトの確認(#2375): 消す / 残す / やめる で何が消えるか、終わった後の状態。 */
class AccountSignOut2375Test {
    private val credential = StoredCredential("uid-1", "refresh-secret")
    private lateinit var tokens: InMemoryTokenStore
    private var storeWipes = 0
    private lateinit var hub: AppHub

    @Before
    fun setUp() {
        tokens = InMemoryTokenStore(credential)
        storeWipes = 0
        hub = AppHub(wipeStore = { storeWipes++ }, clearTempFiles = {})
        DesktopAccount.tokens = tokens
        DesktopAccount.state = AccountState.SignedIn("山田 花子", "hanako@example.com", anonymous = false)
    }

    @After
    fun tearDown() {
        DesktopAccount.tokens = null
        DesktopAccount.state = AccountState.SignedOut
    }

    @Test
    fun wipeErasesDeviceDataAndTokens() = runBlocking {
        val records = hub.acquire("shopping").records
        val before = DesktopAccount.signOuts
        assertTrue(DesktopAccount.signOut(SignOutChoice.WIPE, hub::wipeLocalData))
        assertEquals(1, storeWipes)
        assertEquals("開いているアプリの記録も消して画面を戻す", 1, records.wipes)
        assertNull(tokens.load())
        assertEquals(AccountState.SignedOut, DesktopAccount.state)
        assertEquals(before + 1, DesktopAccount.signOuts)
    }

    @Test
    fun keepErasesOnlyTokens() = runBlocking {
        val records = hub.acquire("shopping").records
        val before = DesktopAccount.signOuts
        assertTrue(DesktopAccount.signOut(SignOutChoice.KEEP, hub::wipeLocalData))
        assertEquals(0, storeWipes)
        assertEquals(0, records.wipes)
        assertNull(tokens.load())
        assertEquals(AccountState.SignedOut, DesktopAccount.state)
        assertEquals("ウィンドウは最初の画面に戻す", before + 1, DesktopAccount.signOuts)
    }

    @Test
    fun cancelChangesNothing() = runBlocking {
        val before = DesktopAccount.signOuts
        assertFalse(DesktopAccount.signOut(SignOutChoice.CANCEL, hub::wipeLocalData))
        assertEquals(0, storeWipes)
        assertEquals(credential, tokens.load())
        assertTrue(DesktopAccount.state is AccountState.SignedIn)
        assertEquals(before, DesktopAccount.signOuts)
    }

    @Test
    fun failedWipeKeepsTokens() = runBlocking {
        val r = runCatching { SignOutCleanup.run(SignOutChoice.WIPE, { error("disk") }, tokens) }
        assertTrue(r.isFailure)
        assertNotNull("消せなかったらサインインのまま", tokens.load())
    }

    @Test
    fun signOutButtonOpensTheConfirmAndWindowsReturnToStart() = runBlocking {
        val app = hub.acquire("shopping")
        val s = DesktopSession(app.dsl, MockStore(), shared = app.records)
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        assertNull("消す口が無ければ差し込まれた物のまま", k.accountActions(AccountActions()).signOut)
        k.wipeLocalData = hub::wipeLocalData
        k.accountActions(AccountActions()).signOut!!.invoke()
        assertTrue(k.signOutConfirmOpen)
        k.chooseSignOut(SignOutChoice.KEEP)
        assertFalse(k.signOutConfirmOpen)
        assertNull(tokens.load())
        assertEquals(0, storeWipes)

        val start = s.top.screen
        s.returnToStart()
        assertEquals(start, s.top.screen)
    }

    @Test
    fun textsInBothLanguages() {
        val table = StringTable.fromClasspath()
        val keys = listOf(
            "account_sign_out_title", "account_cancel", "desktop_sign_out_message", "desktop_sign_out_anonymous_note",
            "desktop_sign_out_keep", "desktop_sign_out_wipe",
        )
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
        assertEquals("You can't return to an anonymous account. To keep the apps you made, choose \"Continue with Google\" first.", table.get(Lang.EN, "desktop_sign_out_anonymous_note"))
    }
}
