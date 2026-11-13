package jp.kz2601.metamon.desktop.theme

import androidx.compose.runtime.compositionLocalOf

/**
 * 「動きを減らす」の値(#2999)。true の時は明滅・移りのアニメを止める(形は出す)。
 * 値を読む口(OS 設定)は別の Issue。そこが繋がるまでは false(従来どおり動く)。
 */
val LocalReduceMotion = compositionLocalOf { false }

/** 動きを止める所の規則。Compose に依らず試せるようにここに置く。 */
object MotionRules {
    /** 読み込みの形の明るさ。動きを減らす時は明滅の中間で固定する。 */
    fun skeletonAlpha(reduceMotion: Boolean, animated: Float): Float = if (reduceMotion) SKELETON_STATIC_ALPHA else animated

    const val SKELETON_STATIC_ALPHA = 0.65f
}
