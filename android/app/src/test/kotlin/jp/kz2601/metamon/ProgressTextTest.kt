package jp.kz2601.metamon

import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.StreamedComponent
import org.junit.Assert.assertEquals
import org.junit.Test

class ProgressTextTest {
    @Test
    fun formatsEachKind() {
        assertEquals(UiText.Res(R.string.voice_progress_app_tabs, "家計簿", "ホーム・一覧"), progressText(GenerateProgress.App("家計簿", listOf("ホーム", "一覧"), 0)))
        assertEquals(UiText.Res(R.string.voice_progress_component, 2, "合計"), progressText(GenerateProgress.Component(StreamedComponent(0, "home", 1, type = "stat", label = "合計"))))
        assertEquals(UiText.Res(R.string.voice_progress_component, 1, "list"), progressText(GenerateProgress.Component(StreamedComponent(0, "home", 0, type = "list"))))
        assertEquals(UiText.Res(R.string.voice_progress_restart), progressText(GenerateProgress.Restart(0)))
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
