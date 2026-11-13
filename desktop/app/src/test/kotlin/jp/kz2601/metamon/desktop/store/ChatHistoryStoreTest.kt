package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.desktop.store.ChatHistoryEntry.Kind
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 会話の履歴: 保存・読み直し・上限での切り詰め・アプリごとの削除・端末のデータを消す。 */
class ChatHistoryStoreTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun entries(n: Int) = (1..n).map { ChatHistoryEntry(Kind.USER, "m$it", id = "id$it") }

    @Test
    fun savesAndReloadsFromDisk() {
        val dir = tmp.root.resolve("chat_history")
        val saved = listOf(
            ChatHistoryEntry(Kind.USER, "家計簿を作って", id = "1"),
            ChatHistoryEntry(Kind.ASSISTANT, "作りました", id = "2"),
            ChatHistoryEntry(Kind.APPLIED, "合計を追加", version = 3, id = "3"),
        )
        ChatHistoryStore(dir).save(saved, "app/1")
        assertTrue("appId はファイル名に使える形にする", dir.resolve("app_1.json").isFile)
        assertEquals("起動し直しても読める", saved, ChatHistoryStore(dir).load("app/1"))
        assertEquals(emptyList<ChatHistoryEntry>(), ChatHistoryStore(dir).load("other"))
    }

    @Test
    fun trimsToLimit() {
        val dir = tmp.root.resolve("chat_history")
        ChatHistoryStore(dir).save(entries(ChatHistoryStore.LIMIT + 5), "a")
        val loaded = ChatHistoryStore(dir).load("a")
        assertEquals(ChatHistoryStore.LIMIT, loaded.size)
        assertEquals("古い方から捨てる", "m6", loaded.first().text)
        assertEquals("m${ChatHistoryStore.LIMIT + 5}", loaded.last().text)
    }

    @Test
    fun removeIsPerApp() {
        val dir = tmp.root.resolve("chat_history")
        val store = ChatHistoryStore(dir)
        store.save(entries(2), "a")
        store.save(entries(3), "b")
        store.remove("a")
        assertEquals(emptyList<ChatHistoryEntry>(), store.load("a"))
        assertFalse(dir.resolve("a.json").exists())
        assertEquals(3, ChatHistoryStore(dir).load("b").size)
        store.save(emptyList(), "b")
        assertFalse("空で保存したらファイルも消す", dir.resolve("b.json").exists())
    }

    @Test
    fun brokenFileIsDropped() {
        val dir = tmp.root.resolve("chat_history").apply { mkdirs() }
        dir.resolve("a.json").writeText("{not json")
        assertEquals(emptyList<ChatHistoryEntry>(), ChatHistoryStore(dir).load("a"))
        assertFalse(dir.resolve("a.json").exists())
    }

    @Test
    fun unknownKindIsSkipped() {
        val text = """[{"id":"1","kind":"user","text":"a"},{"id":"2","kind":"future","text":"b"}]"""
        assertEquals(listOf(ChatHistoryEntry(Kind.USER, "a", id = "1")), ChatHistoryStore.decode(text))
    }

    @Test
    fun deviceWipeRemovesHistory() = runBlocking {
        val store = DesktopStore(tmp.root)
        store.chatHistory.save(entries(2), "a")
        assertTrue(tmp.root.resolve("chat_history/a.json").isFile)
        store.wipe()
        assertFalse(tmp.root.resolve("chat_history").exists())
        assertEquals(emptyList<ChatHistoryEntry>(), store.chatHistory.load("a"))
    }

    @Test
    fun memoryOnlyWhenNoDirectory() {
        val store = ChatHistoryStore(null)
        store.save(entries(1), "a")
        assertEquals(1, store.load("a").size)
        store.removeAll()
        assertEquals(0, store.load("a").size)
    }
}
