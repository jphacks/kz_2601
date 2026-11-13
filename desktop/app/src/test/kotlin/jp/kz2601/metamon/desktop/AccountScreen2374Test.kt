package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** アカウントの画面の枠(#2374): 状態ごとのボタン・状態の行・読み上げ・トレイの入り口。 */
class AccountScreen2374Test {
    private val google = AccountState.SignedIn("山田 花子", "hanako@example.com", anonymous = false)
    private val anonymous = AccountState.SignedIn(null, null, anonymous = true)

    @Test
    fun buttonsPerState() {
        assertEquals(listOf(AccountButton.START_ANONYMOUS, AccountButton.SIGN_IN_GOOGLE), AccountScreenModel.buttons(AccountState.SignedOut))
        assertEquals(listOf(AccountButton.LINK_GOOGLE, AccountButton.SIGN_OUT), AccountScreenModel.buttons(anonymous))
        assertEquals(listOf(AccountButton.SIGN_OUT), AccountScreenModel.buttons(google))
    }

    @Test
    fun statusLinesPerState() = Strings.withLang(Lang.JA) {
        assertEquals(listOf("サインインしていません"), AccountScreenModel.statusLines(AccountState.SignedOut))
        assertEquals(listOf("匿名で使っています"), AccountScreenModel.statusLines(anonymous))
        assertEquals(listOf("山田 花子", "hanako@example.com"), AccountScreenModel.statusLines(google))
        // 匿名は名前・メールが来ても出さない
        assertEquals(listOf("匿名で使っています"), AccountScreenModel.statusLines(AccountState.SignedIn("x", "x@example.com", anonymous = true)))
        // 名前もメールも無ければ「サインインしています」
        assertEquals(listOf("サインインしています"), AccountScreenModel.statusLines(AccountState.SignedIn(" ", null, anonymous = false)))
        assertEquals(listOf("a@example.com"), AccountScreenModel.statusLines(AccountState.SignedIn("a@example.com", "a@example.com", anonymous = false)))
    }

    @Test
    fun accessibilityReadsTheStatusInOneSentence() {
        assertEquals("アカウント: 山田 花子、hanako@example.com", Strings.withLang(Lang.JA) { A11y.accountStatus(AccountScreenModel.statusLines(google)) })
        assertEquals("Account: Not signed in", Strings.withLang(Lang.EN) { A11y.accountStatus(AccountScreenModel.statusLines(AccountState.SignedOut)) })
    }

    @Test
    fun buttonsCallTheGivenFunctionsAndUnsetOnesAreOff() {
        val called = mutableListOf<AccountButton>()
        val actions = AccountActions(startAnonymous = { called += AccountButton.START_ANONYMOUS }, signOut = { called += AccountButton.SIGN_OUT })
        actions.of(AccountButton.START_ANONYMOUS)!!.invoke()
        actions.of(AccountButton.SIGN_OUT)!!.invoke()
        assertEquals(listOf(AccountButton.START_ANONYMOUS, AccountButton.SIGN_OUT), called)
        assertNull(actions.of(AccountButton.SIGN_IN_GOOGLE))
        assertTrue(AccountButton.entries.all { AccountActions().of(it) == null })
    }

    @Test
    fun allTextsExistInBothLanguages() {
        val table = StringTable.fromClasspath()
        val keys = AccountButton.entries.map { it.titleKey } +
            listOf("account_link_status_anonymous", "desktop_account_signed_in", "desktop_account_signed_out", "desktop_a11y_account")
        for (lang in Lang.entries) for (key in keys) assertTrue("$lang $key", table.hasString(lang, key))
        assertEquals(
            listOf("ログインせずに使う(匿名)", "Google でログイン", "Google で続ける", "ログアウト"),
            Strings.withLang(Lang.JA) { AccountButton.entries.map { it.title } },
        )
    }

    @Test
    fun trayOpensTheAccountScreenInTheFrontWindow() = Strings.withLang(Lang.JA) {
        val items = TrayMenu.items("家計簿", canQuickRecord = true, timers = emptyList(), closeToTray = true, account = true)
        val account = items.filterIsInstance<TrayItem.Command>().single { it.command == TrayCommand.ACCOUNT }
        assertEquals("アカウント", account.label)
        assertFalse(TrayMenu.items("家計簿", true, emptyList(), true).any { it is TrayItem.Command && it.command == TrayCommand.ACCOUNT })

        val c = TrayController(Examples.load("kakeibo"), true, MemoryBoolSetting(true), MemoryBoolSetting(true))
        assertFalse(c.takeAccount { true })
        c.openAccount()
        assertEquals(1, c.accountRequests)
        assertFalse("後ろのウィンドウは受け取らない", c.takeAccount { false })
        assertTrue(c.takeAccount { true })
        assertFalse("1 回だけ", c.takeAccount { true })
    }
}
