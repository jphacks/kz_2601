package jp.kz2601.metamon.desktop.theme

import org.junit.Assert.assertEquals
import org.junit.Test

/** 動きを減らす時の読み込みの形の明るさ(#2999)。 */
class ReduceMotionTest {
    @Test
    fun staticWhenReduced() {
        assertEquals(MotionRules.SKELETON_STATIC_ALPHA, MotionRules.skeletonAlpha(true, 0.4f), 0f)
        assertEquals(MotionRules.SKELETON_STATIC_ALPHA, MotionRules.skeletonAlpha(true, 0.9f), 0f)
    }

    @Test
    fun followsAnimationOtherwise() {
        assertEquals(0.4f, MotionRules.skeletonAlpha(false, 0.4f), 0f)
        assertEquals(0.9f, MotionRules.skeletonAlpha(false, 0.9f), 0f)
    }
}
