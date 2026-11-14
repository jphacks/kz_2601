package jp.kz2601.metamon.desktop.voice

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 止めた依頼が残る / 作れたら消える / サインアウトで消える(#3007)。 */
class HeldBuildTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private val held = HeldBuild(
        text = "家計簿を作って",
        reason = HeldBuild.Reason.DAILY_LIMIT,
        chat = listOf(HeldChatLine(true, "家計簿"), HeldChatLine(false, "月ごとに見ますか")),
        dailyFreeLimit = 3,
    )

    private fun store() = HeldBuildStore(tmp.root.resolve("held/build.json"))

    @Test
    fun reasonFollowsHttpStatus() {
        assertEquals(HeldBuild.Reason.DAILY_LIMIT, HeldBuild.reasonFor(429))
        assertEquals(HeldBuild.Reason.PRO_REQUIRED, HeldBuild.reasonFor(402))
        assertNull(HeldBuild.reasonFor(500))
        assertNull(HeldBuild.reasonFor(200))
    }

    @Test
    fun heldRemainsAcrossRestart() {
        store().hold(held)
        assertEquals(held, store().current())
    }

    @Test
    fun builtClearsIt() {
        val s = store()
        s.hold(held)
        s.onBuilt()
        assertNull(s.current())
        assertNull(store().current())
    }

    @Test
    fun signOutClearsIt() {
        val s = store()
        s.hold(held)
        s.onSignOut()
        assertNull(s.current())
        assertFalse(tmp.root.resolve("held/build.json").exists())
        assertNull(store().current())
    }

    @Test
    fun dismissAndSpeakingAgainClearIt() {
        val s = store()
        s.hold(held)
        s.dismiss()
        assertNull(store().current())
    }

    @Test
    fun newHoldReplacesTheOldOne() {
        val s = store()
        s.hold(held)
        val next = HeldBuild("筋トレ記録を作って", HeldBuild.Reason.PRO_REQUIRED)
        s.hold(next)
        assertEquals(next, store().current())
    }

    @Test
    fun blankRequestIsNotKept() {
        val s = store()
        s.hold(held.copy(text = "  "))
        assertNull(s.current())
        assertFalse(tmp.root.resolve("held/build.json").exists())
    }

    @Test
    fun brokenFileIsDroppedAndStartsEmpty() {
        val f = tmp.root.resolve("held/build.json")
        f.parentFile.mkdirs()
        f.writeText("{こわれた")
        assertNull(HeldBuildStore(f).current())
        assertFalse(f.exists())
    }

    @Test
    fun memoryOnlyWhenNoFile() {
        val s = HeldBuildStore(null)
        s.hold(held)
        assertEquals(held, s.current())
        s.onSignOut()
        assertNull(s.current())
    }
}
