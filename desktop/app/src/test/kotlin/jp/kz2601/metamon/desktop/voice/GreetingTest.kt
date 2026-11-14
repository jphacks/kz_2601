package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.voice.Greeting.Context
import jp.kz2601.metamon.desktop.voice.Greeting.HomeEntry
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 起動時の挨拶: Android の `Greeting`・`LaunchVisits` と同じ規則(#3019)。 */
class GreetingTest {
    @Test
    fun contextFollowsEntryAndVisits() {
        assertEquals(Context.FIRST_VISIT, Greeting.context(HomeEntry.LAUNCH, isInitial = true, launchIsReturning = false))
        assertEquals(Context.RETURNING, Greeting.context(HomeEntry.LAUNCH, isInitial = true, launchIsReturning = true))
        assertEquals(Context.NEW_APP, Greeting.context(HomeEntry.NEW_APP, isInitial = true, launchIsReturning = true))
        assertEquals(Context.HOME, Greeting.context(HomeEntry.HOME, isInitial = true, launchIsReturning = false))
        // 初期画面でなければ(既にアプリがある)どの経路でも EDITING
        for (entry in HomeEntry.entries) {
            assertEquals(Context.EDITING, Greeting.context(entry, isInitial = false, launchIsReturning = true))
        }
    }

    @Test
    fun timeOfDayBoundaries() {
        assertEquals("voice_hello_evening", Greeting.timeOfDayKey(4))
        assertEquals("voice_hello_morning", Greeting.timeOfDayKey(5))
        assertEquals("voice_hello_morning", Greeting.timeOfDayKey(10))
        assertEquals("voice_hello_day", Greeting.timeOfDayKey(11))
        assertEquals("voice_hello_day", Greeting.timeOfDayKey(17))
        assertEquals("voice_hello_evening", Greeting.timeOfDayKey(18))
        assertEquals("voice_hello_evening", Greeting.timeOfDayKey(0))
        assertEquals("voice_hello_evening", Greeting.timeOfDayKey(23))
    }

    @Test
    fun copyUsesHelloOnlyWhereAndroidDoes() {
        assertTrue(Greeting.copy(Context.FIRST_VISIT).subtitleHasHello)
        assertTrue(Greeting.copy(Context.RETURNING).subtitleHasHello)
        assertTrue(Greeting.copy(Context.HOME).subtitleHasHello)
        assertFalse(Greeting.copy(Context.NEW_APP).subtitleHasHello)
        assertFalse(Greeting.copy(Context.EDITING).subtitleHasHello)
        assertEquals("voice_title_first", Greeting.copy(Context.FIRST_VISIT).titleKey)
        assertEquals("voice_subtitle_editing", Greeting.copy(Context.EDITING).subtitleKey)
    }

    @Test
    fun launchVisitsCountsOnceAndStaysForTheProcess() {
        assertFalse(LaunchVisits.note(0))
        assertTrue(LaunchVisits.note(1))
        val store = InMemoryKeyValueStore()
        val first = LaunchVisits(store)
        assertFalse(first.isReturning())
        assertFalse(first.isReturning())
        assertEquals(1, store.getInt(LaunchVisits.KEY))
        // 次の起動(別のプロセス)は再訪
        val second = LaunchVisits(store)
        assertTrue(second.isReturning())
        assertTrue(second.isReturning())
        assertEquals(2, store.getInt(LaunchVisits.KEY))
    }

    @Test
    fun forgetKeepsThisLaunchButResetsNext() {
        val store = InMemoryKeyValueStore()
        LaunchVisits(store).isReturning()
        val second = LaunchVisits(store)
        assertTrue(second.isReturning())
        second.forget()
        assertTrue(second.isReturning())
        assertFalse(LaunchVisits(store).isReturning())
    }

    @Test
    fun textInBothLanguages() {
        Strings.withLang(Lang.JA) {
            val (title, sub) = Greeting.text(Context.FIRST_VISIT, hour = 8)
            assertEquals("どんなアプリが欲しいですか?", title)
            assertEquals("おはようございます。話しかけるだけで、アプリが生まれ変わります", sub)
            assertEquals("話しかけると、このアプリが育ちます", Greeting.text(Context.EDITING, hour = 8).second)
        }
        Strings.withLang(Lang.EN) {
            val (title, sub) = Greeting.text(Context.RETURNING, hour = 20)
            assertEquals("Welcome back.\nWhat shall we make today?", title)
            assertEquals("Good evening. Long-press the mic to bring back an app you made", sub)
            assertEquals("Hello. Just talk, and the app transforms", Greeting.text(Context.FIRST_VISIT, hour = 12).second)
        }
    }
}
