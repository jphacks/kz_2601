package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #891: プロセスが落とされた後、同じアプリの同じ画面に戻す。戻せない時は開いた直後の画面。 */
class NavigationRestoreTest {
    private val kakeibo = RuntimeFixture.dsl("kakeibo")
    private val warikan = RuntimeFixture.dsl("warikan")

    private val deep = NavigationState(
        paths = mapOf(Route.tabStack("stats") to listOf(Route("expense_form", recordId = "r1", stack = Route.tabStack("stats")))),
        selectedTab = "stats",
        sheet = Route("expense_form", stack = Route.SHEET),
    )

    @Test
    fun encodeAndDecodeRoundTrip() {
        val snapshot = NavigationSnapshot("app-1", deep)
        assertEquals(snapshot, NavigationSnapshot.decode(snapshot.encode()))
    }

    @Test
    fun brokenTextIsIgnored() {
        assertNull(NavigationSnapshot.decode("{not json"))
        assertNull(NavigationSnapshot.decode(null))
    }

    @Test
    fun restoresAfterBootstrapWhenTheAppMatches() = runTest {
        // 前のプロセスで保存した位置 → 新しいプロセスのランタイム
        val saved = NavigationSnapshot("test-app", deep).encode()
        val rt = runtime(kakeibo)
        rt.offerNavigationRestore(NavigationSnapshot.decode(saved))
        rt.bootstrap()
        assertEquals(deep, rt.navigation.value)
        assertEquals(NavigationSnapshot("test-app", deep), rt.navigationSnapshot())
    }

    @Test
    fun otherAppStartsFromTheFirstScreen() = runTest {
        val rt = runtime(kakeibo)
        rt.offerNavigationRestore(NavigationSnapshot("another-app", deep))
        rt.bootstrap()
        assertEquals(NavigationState.initial(kakeibo), rt.navigation.value)
    }

    @Test
    fun screensMissingFromTheDslStartFromTheFirstScreen() = runTest {
        // 背面の間に DSL が変わって画面が無くなった(warikan にタブ・stats は無い)
        val rt = runtime(warikan)
        rt.offerNavigationRestore(NavigationSnapshot("test-app", deep))
        rt.bootstrap()
        assertEquals(NavigationState.initial(warikan), rt.navigation.value)
    }

    @Test
    fun recreatingTheScreenDoesNotOverwriteWhereTheUserWent() = runTest {
        // 回転など: ランタイムは生きていて、もう別の画面にいる
        val rt = runtime(kakeibo)
        rt.bootstrap()
        val moved = NavigationState.initial(kakeibo).copy(selectedTab = "stats")
        rt.setNavigation(moved)
        rt.offerNavigationRestore(NavigationSnapshot("test-app", deep))
        assertEquals(moved, rt.navigation.value)
    }

    @Test
    fun voiceOnlyScreenSavesNothing() = runTest {
        val rt = runtime(RuntimeFixture.dsl("initial"))
        assertNull(rt.navigationSnapshot())
    }
}
