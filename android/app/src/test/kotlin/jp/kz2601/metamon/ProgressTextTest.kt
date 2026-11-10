package jp.kz2601.metamon

import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.StreamedComponent
import org.junit.Assert.assertEquals
import org.junit.Test

class ProgressTextTest {
    @Test
    fun formatsEachKind() {
        assertEquals("「家計簿」を作っています(ホーム・一覧)", progressText(GenerateProgress.App("家計簿", listOf("ホーム", "一覧"), 0)))
        assertEquals("部品 2: 合計", progressText(GenerateProgress.Component(StreamedComponent(0, "home", 1, type = "stat", label = "合計"))))
        assertEquals("部品 1: list", progressText(GenerateProgress.Component(StreamedComponent(0, "home", 0, type = "list"))))
        assertEquals("やり直しています", progressText(GenerateProgress.Restart(0)))
    }

    @Test
    fun releaseKnobsDefaultToNone() {
        // release の DebugLaunch は常に NONE。既定値がモックを向いていないこと
        val k = DebugKnobs.NONE
        assertEquals(false, k.mock)
        assertEquals(null, k.autoTranscript)
        assertEquals(emptyList<DebugKnobs.AutoNavStep>(), k.autoNav)
    }
}
