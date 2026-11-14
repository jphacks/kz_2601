package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.feedback.OverlayPlan.Step
import org.junit.Assert.assertEquals
import org.junit.Test

class OverlayPlanTest {
    private fun layer(left: Int = 0, top: Int = 0, width: Int = 100, height: Int = 200, secure: Boolean = false, dim: Float = 0f) =
        WindowLayer(left, top, width, height, secure, dim)

    @Test
    fun drawsInOrderRelativeToBase() {
        val steps = OverlayPlan.plan(0, 50, 100, 200, listOf(layer(top = 50), layer(left = 10, top = 150, width = 80, height = 100)))
        assertEquals(listOf(Step.Draw(0, 0, 0), Step.Draw(1, 10, 100)), steps)
    }

    @Test
    fun dimsBeforeTheDialog() {
        val steps = OverlayPlan.plan(0, 0, 100, 200, listOf(layer(left = 20, top = 60, width = 60, height = 80, dim = 0.6f)))
        assertEquals(listOf(Step.Dim(0.6f), Step.Draw(0, 20, 60)), steps)
    }

    @Test
    fun clampsDim() {
        assertEquals(Step.Dim(1f), OverlayPlan.plan(0, 0, 100, 200, listOf(layer(dim = 3f))).first())
        assertEquals(listOf(Step.Draw(0, 0, 0)), OverlayPlan.plan(0, 0, 100, 200, listOf(layer(dim = -1f))))
    }

    @Test
    fun skipsSecureWindowsAndTheirDim() {
        val steps = OverlayPlan.plan(0, 0, 100, 200, listOf(layer(secure = true, dim = 0.5f), layer()))
        assertEquals(listOf(Step.Draw(1, 0, 0)), steps)
    }

    @Test
    fun skipsEmptyAndOutsideWindows() {
        val steps = OverlayPlan.plan(
            0, 0, 100, 200,
            listOf(layer(width = 0), layer(left = 100), layer(top = -200), layer(left = -99, width = 100)),
        )
        assertEquals(listOf(Step.Draw(3, -99, 0)), steps)
    }
}
