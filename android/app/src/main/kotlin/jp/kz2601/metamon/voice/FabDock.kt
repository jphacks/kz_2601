package jp.kz2601.metamon.voice

import android.content.Context

/** FAB を寄せる辺 */
enum class FabSide { LEFT, RIGHT }

/**
 * マイク FAB のピクチャーインピクチャー風の吸着・隠す判定(iOS の `FabDock`・#52)。View から切り離した純粋関数。
 * 座標は FAB を置く領域のローカル座標(左上原点・px)。「下端距離」= 領域の下端から FAB の下端まで。
 */
object FabDock {
    /** 離した時の中心が端からこの割合 × FAB 径 以内なら隠す */
    const val HIDE_FRACTION = 0.3f

    /** 保存値がこれなら「自動」(既定の下端距離) */
    const val AUTO_BOTTOM = -1f

    data class Snap(val side: FabSide, val hidden: Boolean)

    /**
     * 指を離した時の吸着先。辺は予測位置([predictedCenterX]・弾いた方向)で選ぶ。
     * 実位置か予測位置のどちらかが選んだ辺から `fabSize * HIDE_FRACTION` 以内(はみ出しを含む)なら隠す。
     */
    fun snap(centerX: Float, predictedCenterX: Float, width: Float, fabSize: Float): Snap {
        val side = if (predictedCenterX < width / 2) FabSide.LEFT else FabSide.RIGHT
        val threshold = fabSize * HIDE_FRACTION
        fun edge(x: Float) = if (side == FabSide.LEFT) x else width - x
        return Snap(side, edge(centerX) <= threshold || edge(predictedCenterX) <= threshold)
    }

    /** 取っ手を内側(左の取っ手なら右)へ [revealDistance] 以上引いたら戻す */
    fun shouldReveal(translationX: Float, side: FabSide, revealDistance: Float): Boolean =
        (if (side == FabSide.LEFT) translationX else -translationX) >= revealDistance

    /** 表示する下端距離。自動(負値)なら [autoInset]。ユーザー値は [autoInset]〜[maxBottom] に収める */
    fun displayedBottom(stored: Float, autoInset: Float, maxBottom: Float): Float {
        if (stored < 0) return autoInset
        return maxOf(autoInset, minOf(stored, maxBottom))
    }

    /** 離した位置(FAB 中心 y)から保存する下端距離。[minBottom]〜[maxBottom] に収める */
    fun storedBottom(centerY: Float, height: Float, fabSize: Float, minBottom: Float, maxBottom: Float): Float {
        val raw = height - centerY - fabSize / 2
        return maxOf(minBottom, minOf(raw, maxOf(minBottom, maxBottom)))
    }
}

/** FAB を端に隠した時の案内(iOS の `FabHideHint`・L6)。出ていた FAB を初めて隠した時だけ数秒出す */
object FabHideHint {
    const val MILLIS = 4_000L

    fun shouldShow(wasHidden: Boolean, nowHidden: Boolean, alreadyShown: Boolean): Boolean =
        !wasHidden && nowHidden && !alreadyShown
}

/** FAB の位置(辺・下端距離 dp・隠れ)。アプリ共通で端末に残す(iOS の `@AppStorage("fab.*")`) */
class FabPositionStore(context: Context) {
    private val prefs = context.applicationContext.getSharedPreferences("metamon.fab", Context.MODE_PRIVATE)

    var side: FabSide
        get() = if (prefs.getString("fab.side", null) == "right") FabSide.RIGHT else FabSide.LEFT
        set(value) = prefs.edit().putString("fab.side", if (value == FabSide.RIGHT) "right" else "left").apply()

    /** dp。[FabDock.AUTO_BOTTOM] = 自動 */
    var bottomDp: Float
        get() = prefs.getFloat("fab.bottom", FabDock.AUTO_BOTTOM)
        set(value) = prefs.edit().putFloat("fab.bottom", value).apply()

    var hidden: Boolean
        get() = prefs.getBoolean("fab.hidden", false)
        set(value) = prefs.edit().putBoolean("fab.hidden", value).apply()

    var hideHintShown: Boolean
        get() = prefs.getBoolean("fab.hideHintShown", false)
        set(value) = prefs.edit().putBoolean("fab.hideHintShown", value).apply()
}
