package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AutoStart
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.components.dashcam.DashcamAutoStart
import jp.kz2601.metamon.renderer.components.dashcam.DashcamConfig
import jp.kz2601.metamon.renderer.components.dashcam.DashcamImpact
import jp.kz2601.metamon.renderer.components.dashcam.DashcamLoop
import jp.kz2601.metamon.renderer.components.dashcam.DashcamSegment
import jp.kz2601.metamon.renderer.components.dashcam.DashcamStore
import jp.kz2601.metamon.renderer.components.dashcam.DashcamStoreKey
import jp.kz2601.metamon.renderer.components.dashcam.DashcamText
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/** `dashcam`(#872)の純粋ロジックと置き場(iOS の DashcamTests / DashcamLeafTests の写し)。時刻は ms。 */
class DashcamLeavesTest {
    @get:Rule val tmp = TemporaryFolder()

    private val t0 = 1_800_000_000_000L
    private fun at(s: Double): Long = t0 + (s * 1000).toLong()
    private fun seg(start: Double, mb: Long = 50, saved: Boolean = false) =
        DashcamSegment(at(start), 60_000, mb * 1_000_000, saved)

    @Test
    fun registeredAndDeclared() {
        assertNotNull(ComponentRegistry.standard.builder(ComponentType.DASHCAM))
    }

    // MARK: - config

    @Test
    fun configDefaultsAndClamps() {
        val d = DashcamConfig.of()
        assertEquals(600_000L, d.keepMillis)
        assertEquals(2_000_000_000L, d.maxBytes)
        assertEquals(2.5, d.impactG, 0.0)
        assertTrue(d.autoStart.isEmpty())
        assertFalse(d.audio)

        val c = DashcamConfig.of(999, 1, 0.1, listOf(AutoStart.LANDSCAPE, AutoStart.UNKNOWN, AutoStart.CHARGING), true)
        assertEquals(3_600_000L, c.keepMillis)
        assertEquals(200_000_000L, c.maxBytes)
        assertEquals(1.2, c.impactG, 0.0)
        assertEquals("未知の条件は無視", setOf(AutoStart.LANDSCAPE, AutoStart.CHARGING), c.autoStart)
        assertTrue(c.audio)
        assertEquals(8.0, DashcamConfig.of(impactG = 99.0).impactG, 0.0)
        assertEquals(20_000_000_000L, DashcamConfig.of(maxStorageMB = 999_999).maxBytes)
        assertEquals(60_000L, DashcamConfig.of(keepMinutes = 0).keepMillis)
    }

    @Test
    fun configFromComponent() {
        val comp = Component(
            type = ComponentType.DASHCAM, keepMinutes = 5, maxStorageMB = 1000, impactG = 3.0,
            autoStart = listOf(AutoStart.LANDSCAPE, AutoStart.STILL), audio = true,
        )
        val c = DashcamConfig.of(comp)
        assertEquals(300_000L, c.keepMillis)
        assertEquals(1_000_000_000L, c.maxBytes)
        assertEquals(3.0, c.impactG, 0.0)
        assertEquals(setOf(AutoStart.LANDSCAPE, AutoStart.STILL), c.autoStart)
        assertTrue(c.audio)
    }

    // MARK: - segment names

    @Test
    fun segmentFileNameRoundTrip() {
        val s = DashcamSegment(at(0.25), 59_500, 10, saved = true)
        assertEquals("seg-1800000000250-59500.mp4", s.fileName)
        assertEquals(s, DashcamSegment.parse(s.fileName, 10, saved = true))
        for (bad in listOf("rec-1800000000250.mp4", "seg-1-2.mov", "seg-abc-1.mp4", "seg-1.mp4", "seg--1-2.mp4", "../seg-1-2.mp4", "seg-1-2-3.mp4")) {
            assertNull(bad, DashcamSegment.parse(bad))
        }
    }

    // MARK: - loop

    /** 直近 10 分だけ残す: 終わりが 10 分より前の区間を古い順に消す。保存済みは消さない。 */
    @Test
    fun loopKeepsOnlyRecentMinutes() {
        val segs = (0 until 13).map { seg(it * 60.0, saved = it == 2) }
        val drop = DashcamLoop.expired(segs, now = at(780.0), keepMillis = 600_000, maxBytes = Long.MAX_VALUE)
        // cutoff = 180: end < 180 は 0(60)・60(120) の 2 つ。120(180) は境界で残す
        assertEquals(listOf(at(0.0), at(60.0)), drop.map { it.start })
    }

    /** 容量: 合計が上限を超える分を古いループ区間から消す。 */
    @Test
    fun loopRespectsStorageCap() {
        val segs = listOf(seg(0.0), seg(60.0, saved = true), seg(120.0), seg(180.0), seg(240.0)) // 50MB × 5
        val drop = DashcamLoop.expired(segs, now = at(300.0), keepMillis = 3_600_000, maxBytes = 160_000_000)
        assertEquals("保存済み(60)を飛ばして古い順に 2 つ消すと 150MB", listOf(at(0.0), at(120.0)), drop.map { it.start })
        assertTrue(drop.none { it.saved })
    }

    @Test
    fun fullWhenSavedAloneReachesCap() {
        val segs = listOf(seg(0.0, saved = true), seg(60.0, saved = true), seg(120.0))
        assertFalse(DashcamLoop.isFull(segs, maxBytes = 200_000_000, reserve = 50_000_000))
        assertTrue(DashcamLoop.isFull(segs, maxBytes = 140_000_000, reserve = 50_000_000))
        assertEquals(50_000_000L, DashcamLoop.reserve(segs))
        assertEquals(60_000_000L, DashcamLoop.reserve(emptyList()))
    }

    @Test
    fun storageProblem() {
        val segs = listOf(seg(0.0, saved = true), seg(60.0, saved = true))
        assertEquals("端末の空きが少ない", true, DashcamStore.storageProblem(segs, freeBytes = 100_000_000, maxBytes = Long.MAX_VALUE))
        assertEquals("保存済みで上限", false, DashcamStore.storageProblem(segs, freeBytes = null, maxBytes = 120_000_000))
        assertNull(DashcamStore.storageProblem(segs, freeBytes = 10_000_000_000, maxBytes = 2_000_000_000))
    }

    // MARK: - impact

    @Test
    fun impactThresholdAndCooldown() {
        val d = DashcamImpact(2.5)
        assertFalse("√3 ≈ 1.7G は衝撃ではない", d.feed(1.0, 1.0, 1.0, at(10.0)))
        assertTrue("2.5G で衝撃", d.feed(2.0, 1.5, 0.0, at(100.0)))
        assertFalse("5 秒以内の揺れ戻しは数えない", d.feed(0.0, 0.0, -4.0, at(102.0)))
        assertTrue(d.feed(0.0, 0.0, -4.0, at(106.0)))
        assertEquals(2, d.windows.size)
        assertEquals(at(106.0), d.lastImpact)
    }

    /** 衝撃(100 秒)の前 60 秒〜後 30 秒にかかる区間を保存済みにする。 */
    @Test
    fun impactProtectsSurroundingSegments() {
        val d = DashcamImpact(2.5)
        d.feed(3.0, 0.0, 0.0, at(100.0))
        val protected = listOf(0.0, 60.0, 120.0, 180.0).filter { d.protects(at(it), at(it + 60)) }
        assertEquals("窓は 40〜130 秒", listOf(0.0, 60.0, 120.0), protected)
        d.forget(before = at(200.0))
        assertTrue(d.windows.isEmpty())
    }

    @Test
    fun manualMarkProtectsLikeImpact() {
        val d = DashcamImpact(8.0)
        d.mark(at(500.0))
        assertTrue(d.protects(at(480.0), at(540.0)))
        assertFalse(d.protects(at(300.0), at(360.0)))
    }

    // MARK: - auto start

    private fun sample(s: Double, landscape: Boolean = true, still: Boolean = true, charging: Boolean = false) =
        DashcamAutoStart.Sample(
            gx = if (landscape) -0.95 else 0.05,
            gy = if (landscape) 0.1 else -0.98,
            gz = if (landscape) -0.3 else -0.1,
            acceleration = if (still) 0.01 else 0.3,
            rotation = if (still) 0.05 else 1.0,
            charging = charging,
            at = at(s),
        )

    @Test
    fun autoStartAfterHoldThenOnlyOnce() {
        val a = DashcamAutoStart(setOf(AutoStart.LANDSCAPE, AutoStart.STILL))
        assertFalse(a.feed(sample(0.0)))
        assertEquals(3, a.remainingSeconds(at(2.0)))
        assertFalse(a.feed(sample(4.9)))
        assertTrue("5 秒続いたら開始", a.feed(sample(5.0)))
        assertFalse("開始した後は 2 回目を出さない", a.feed(sample(30.0)))
        assertNull(a.remainingSeconds(at(30.0)))
    }

    @Test
    fun autoStartResetsWhenConditionBreaks() {
        val a = DashcamAutoStart(setOf(AutoStart.LANDSCAPE, AutoStart.STILL))
        a.feed(sample(0.0))
        assertFalse("持ち上げたら数え直し", a.feed(sample(3.0, still = false)))
        assertFalse(a.feed(sample(4.0)))
        assertFalse(a.feed(sample(8.5)))
        assertTrue(a.feed(sample(9.0)))
    }

    /** 手で止めた後は、条件が一度外れて(持ち上げて)また置くまで始めない。 */
    @Test
    fun manualStopDisarmsUntilConditionsBreak() {
        val a = DashcamAutoStart(setOf(AutoStart.LANDSCAPE))
        a.disarm()
        assertFalse(a.feed(sample(0.0)))
        assertFalse(a.feed(sample(10.0)))
        assertFalse(a.feed(sample(11.0, landscape = false)))
        assertFalse(a.feed(sample(12.0)))
        assertTrue(a.feed(sample(17.0)))
    }

    @Test
    fun chargingAndNoConditions() {
        val a = DashcamAutoStart(setOf(AutoStart.CHARGING))
        assertFalse(a.feed(sample(0.0, landscape = false, still = false)))
        assertFalse("充電していなければ始めない", a.feed(sample(10.0, landscape = false, still = false)))
        assertFalse(a.feed(sample(11.0, landscape = false, still = false, charging = true)))
        assertTrue(a.feed(sample(16.0, landscape = false, still = false, charging = true)))

        val manual = DashcamAutoStart(emptySet())
        assertFalse(manual.feed(sample(0.0)))
        assertFalse("条件なし = 手動だけ", manual.feed(sample(100.0)))
    }

    @Test
    fun landscapeFromGravity() {
        assertTrue(DashcamAutoStart.isLandscape(0.9, 0.2))
        assertTrue("スタンドで後ろに傾いていても横向き", DashcamAutoStart.isLandscape(-0.6, 0.1))
        assertFalse("縦向き", DashcamAutoStart.isLandscape(0.1, -0.95))
        assertFalse("机に平置き", DashcamAutoStart.isLandscape(0.1, 0.1))
    }

    // MARK: - store

    @Test
    fun storeKeyMatchesIosAndIsPathSafe() {
        assertEquals("iOS と同じ FNV-1a(置き場の名前を揃える)", "876d4e56d7b029ef", DashcamStoreKey.make("置くだけドラレコ"))
        assertEquals("cbf29ce484222325", DashcamStoreKey.make(""))
        val k = DashcamStoreKey.make("置くだけ/ドラレコ ../")
        assertNotEquals(k, DashcamStoreKey.make("別のアプリ"))
        assertTrue(k.all { it in '0'..'9' || it in 'a'..'f' })
    }

    @Test
    fun storeFinalizeSaveDelete() {
        val base = tmp.newFolder("dashcam")
        val store = DashcamStore(base, "app")
        store.loopDir.mkdirs()
        val stale = File(store.loopDir, DashcamSegment.recordingName(t0 - 1))
        stale.writeBytes(ByteArray(5))
        store.prepare()
        assertFalse("閉じられなかった録画中のファイルは消す", stale.exists())

        val r1 = store.recordingFile(t0).apply { writeBytes(ByteArray(1000)) }
        val s1 = store.finalize(r1, t0, 60_000, saved = false)!!
        val r2 = store.recordingFile(at(60.0)).apply { writeBytes(ByteArray(2000)) }
        val s2 = store.finalize(r2, at(60.0), 60_000, saved = false)!!
        assertNull("ファイルが無ければ区間にしない", store.finalize(store.recordingFile(at(120.0)), at(120.0), 1, saved = false))
        val empty = store.recordingFile(at(180.0)).apply { writeBytes(ByteArray(0)) }
        assertNull("空のファイルは区間にしない", store.finalize(empty, at(180.0), 1, saved = false))
        assertFalse(empty.exists())
        assertEquals(listOf(s1, s2), store.segments())
        assertEquals(listOf(1000L, 2000L), store.segments().map { it.bytes })

        val moved = store.save(s1)!!
        assertTrue(moved.saved)
        assertTrue(store.file(moved).exists())
        assertFalse(store.file(s1).exists())

        store.delete(s2)
        assertEquals(listOf(moved), store.segments())
        assertTrue("保存済みはループで消さない", DashcamLoop.expired(store.segments(), at(9999.0), 60_000, 1).isEmpty())
    }

    @Test
    fun storeIsPerApp() {
        val base = tmp.newFolder("dashcam")
        val a = DashcamStore(base, "A")
        val b = DashcamStore(base, "B")
        assertNotEquals(a.root, b.root)
        assertEquals(base, a.root.parentFile)
        assertEquals(DashcamStoreKey.make("A"), a.root.name)
    }

    // MARK: - text

    @Test
    fun textHelpers() {
        assertEquals("1:02:05", DashcamText.clock(3_725_000))
        assertEquals("00:59", DashcamText.clock(59_900))
        assertEquals("00:00", DashcamText.clock(-5))
        assertEquals(10, DashcamText.keepMinutes(DashcamConfig.of()))
        assertEquals(
            listOf(AutoStart.LANDSCAPE, AutoStart.STILL, AutoStart.CHARGING),
            DashcamText.orderedConditions(setOf(AutoStart.CHARGING, AutoStart.LANDSCAPE, AutoStart.STILL)),
        )
    }

    /** 常に出す説明は「画面を出している間だけ」を日英で言う(背面では録らないことを隠さない)。 */
    @Test
    fun footnoteSaysScreenMustStayOnInBothLanguages() {
        val res = File(requireNotNull(System.getProperty("metamon.rendererSrcDir"))).resolve("../res")
        val ja = res.resolve("values/strings_dashcam.xml").readText()
        val en = res.resolve("values-en/strings_dashcam.xml").readText()
        assertTrue(ja.contains("画面を出している間だけ録画します"))
        assertTrue(en.contains("only while this screen is showing"))
        val names = Regex("""name="([a-z_]+)"""").findAll(ja).map { it.groupValues[1] }.toSet()
        assertEquals("日英で同じ文言の名前", names, Regex("""name="([a-z_]+)"""").findAll(en).map { it.groupValues[1] }.toSet())
    }
}
