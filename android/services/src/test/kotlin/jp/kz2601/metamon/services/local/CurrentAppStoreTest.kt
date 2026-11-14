package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.AppSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

/** 今のアプリの appId の保存(iOS の DeviceAppID)と、一覧の保存・並び(#881)。 */
class CurrentAppStoreTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun minting(vararg ids: String): () -> String {
        val queue = ArrayDeque(ids.toList())
        return { queue.removeFirst() }
    }

    @Test
    fun firstLaunchMintsAndNextLaunchReopensTheSameApp() {
        val dir = tmp.newFolder("metamon-app")
        val first = CurrentAppStore(dir, minting("a1"))
        assertEquals("a1", first.current)
        assertTrue("インストール直後", first.issuedThisLaunch)
        assertEquals("a1", first.current)

        val next = CurrentAppStore(dir, minting("never")) // 別インスタンス = 再起動後
        assertEquals("a1", next.current)
        assertFalse(next.issuedThisLaunch)
    }

    @Test
    fun setAndRotateAreKeptAcrossLaunches() {
        val dir = tmp.newFolder("metamon-app")
        val store = CurrentAppStore(dir, minting("a1", "a2"))
        store.current
        store.set("server-app")
        assertEquals("server-app", CurrentAppStore(dir).current)

        assertEquals("a2", store.rotate())
        assertEquals("a2", CurrentAppStore(dir).current)
    }

    @Test
    fun brokenFileMintsANewAppId() {
        val dir = tmp.newFolder("metamon-app")
        dir.resolve("current.json").writeText("{broken")
        val store = CurrentAppStore(dir, minting("fresh"))
        assertEquals("fresh", store.current)
        assertTrue(store.issuedThisLaunch)
    }

    @Test
    fun appListCacheRoundTrips() {
        val dir = tmp.newFolder("metamon-app")
        assertTrue(AppListCache(dir).load().isEmpty())
        AppListCache(dir).store(listOf(AppSummary("a", "家計簿", 3)))
        assertEquals(listOf(AppSummary("a", "家計簿", 3)), AppListCache(dir).load())
        AppListCache(dir).clear()
        assertTrue(AppListCache(dir).load().isEmpty())
    }

    @Test
    fun visibleDropsArchivedAndBlankAndPutsCurrentFirst() {
        val apps = listOf(
            AppSummary("old", "古い", 3, updatedAt = "2026-11-01T00:00:00Z"),
            AppSummary("new", "新しい", 4, updatedAt = "2026-11-10T00:00:00.123Z"),
            AppSummary("cur", "今", 2, updatedAt = "2026-10-01T00:00:00Z"),
            AppSummary("blank", "", 1, updatedAt = "2026-11-11T00:00:00Z"),
            AppSummary("gone", "", 5, status = "archived"),
            AppSummary("new", "重複", 4),
        )
        assertEquals(listOf("cur", "new", "old"), AppSummary.visible(apps, current = "cur").map { it.id })
        assertEquals("new", AppSummary.latest(apps, excluding = "cur")?.id)
        assertEquals("old", AppSummary.latest(apps, excluding = "new")?.id)
        assertNull(AppSummary.latest(listOf(AppSummary("blank", "", 1)), excluding = "x"))
    }

    @Test
    fun visibleFollowsTheUsersOwnOrder() {
        val apps = listOf(
            AppSummary("b", "", 3, sortIndex = 2),
            AppSummary("a", "", 3, sortIndex = 1),
            AppSummary("added", "", 3, updatedAt = "2026-11-10T00:00:00Z"),
        )
        assertEquals(listOf("added", "a", "b"), AppSummary.visible(apps, current = "b").map { it.id })
    }
}
