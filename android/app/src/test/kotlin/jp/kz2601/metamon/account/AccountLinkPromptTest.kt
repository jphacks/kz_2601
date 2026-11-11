package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.voice.stage.PostBuildNotice
import jp.kz2601.metamon.voice.stage.PostBuildWaiting
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の AccountLinkTests の予約と表示の分(#1316)。 */
class AccountLinkPromptTest {
    private val store = InMemoryKeyValueStore()

    @Test
    fun notShownInTheLaunchThatMadeTheFirstApp() {
        val first = AccountLinkPrompt(store, launch = "launch-1")
        first.offerIfNeeded(isAnonymous = true)
        assertFalse("同じ起動ではまだ出さない", first.presentDeferredIfNeeded(isAnonymous = true))
        assertFalse(first.visible.value)
    }

    @Test
    fun shownOnceOnTheNextLaunch() {
        AccountLinkPrompt(store, launch = "launch-1").offerIfNeeded(isAnonymous = true)
        val next = AccountLinkPrompt(store, launch = "launch-2")
        assertTrue(next.presentDeferredIfNeeded(isAnonymous = true))
        assertTrue(next.visible.value)
        assertTrue(store.getBoolean(AccountLinkPrompt.KEY) == true)
        assertNull("予約は消える", store.getString(AccountLinkPrompt.PENDING_KEY))
        next.dismiss()
        assertFalse(next.visible.value)

        val third = AccountLinkPrompt(store, launch = "launch-3")
        third.offerIfNeeded(isAnonymous = true)
        assertFalse("端末ごとに 1 回だけ", third.presentDeferredIfNeeded(isAnonymous = true))
    }

    @Test
    fun keepsTheEarliestReservation() {
        AccountLinkPrompt(store, launch = "launch-1").offerIfNeeded(isAnonymous = true)
        AccountLinkPrompt(store, launch = "launch-2").offerIfNeeded(isAnonymous = true)
        assertEquals("launch-1", store.getString(AccountLinkPrompt.PENDING_KEY))
    }

    @Test
    fun signedInUsersAreNotOffered() {
        AccountLinkPrompt(store, launch = "launch-1").offerIfNeeded(isAnonymous = false)
        assertNull(store.getString(AccountLinkPrompt.PENDING_KEY))
    }

    @Test
    fun linkedBeforeTheNextLaunchDropsTheReservation() {
        AccountLinkPrompt(store, launch = "launch-1").offerIfNeeded(isAnonymous = true)
        val next = AccountLinkPrompt(store, launch = "launch-2")
        assertFalse(next.presentDeferredIfNeeded(isAnonymous = false))
        assertNull(store.getString(AccountLinkPrompt.PENDING_KEY))
        assertTrue("引き継いだ人には数えない", next.shouldOffer())
    }

    @Test
    fun forgetLetsTheNextPersonSeeItAgain() {
        val prompt = AccountLinkPrompt(store, launch = "launch-1")
        prompt.markOffered()
        assertFalse(prompt.shouldOffer())
        prompt.forget()
        assertTrue(prompt.shouldOffer())
    }

    @Test
    fun comesAfterTheOtherNotices() {
        assertEquals(PostBuildNotice.SUMMARY, PostBuildNotice.current(PostBuildNotice.pending(summary = true, waiting = PostBuildWaiting(accountLink = true))))
        assertEquals(
            PostBuildNotice.RETENTION,
            PostBuildNotice.current(PostBuildNotice.pending(summary = false, retention = true, waiting = PostBuildWaiting(accountLink = true))),
        )
        assertEquals(PostBuildNotice.ACCOUNT_LINK, PostBuildNotice.current(PostBuildNotice.pending(summary = false, waiting = PostBuildWaiting(accountLink = true))))
    }
}
