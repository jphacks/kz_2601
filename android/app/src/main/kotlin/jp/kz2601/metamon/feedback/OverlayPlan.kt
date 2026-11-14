package jp.kz2601.metamon.feedback

/** 重ねる窓 1 つ。位置は画面の座標(px)。[dimBehind] は FLAG_DIM_BEHIND の時の暗さ(なければ 0)。 */
internal data class WindowLayer(
    val left: Int,
    val top: Int,
    val width: Int,
    val height: Int,
    val secure: Boolean,
    val dimBehind: Float,
)

/** Activity の窓の上に、シート・ダイアログの窓をどう重ねるか(Android の型に触らない計算だけ)。 */
internal object OverlayPlan {
    sealed interface Step {
        /** ここまでの絵を黒で暗くする */
        data class Dim(val alpha: Float) : Step

        /** layers[index] の窓を、元の絵の (dx, dy) に置く */
        data class Draw(val index: Int, val dx: Int, val dy: Int) : Step
    }

    /**
     * 下から順に並んだ [layers] を、(baseLeft, baseTop) にある幅 baseWidth・高さ baseHeight の絵に重ねる手順。
     * FLAG_SECURE の窓は写さない(その窓の暗がりも足さない)。大きさが 0 の窓・絵の外にある窓も飛ばす。
     */
    fun plan(baseLeft: Int, baseTop: Int, baseWidth: Int, baseHeight: Int, layers: List<WindowLayer>): List<Step> {
        val steps = mutableListOf<Step>()
        layers.forEachIndexed { i, layer ->
            if (layer.secure || layer.width <= 0 || layer.height <= 0) return@forEachIndexed
            val dx = layer.left - baseLeft
            val dy = layer.top - baseTop
            val outside = dx >= baseWidth || dy >= baseHeight || dx + layer.width <= 0 || dy + layer.height <= 0
            if (outside) return@forEachIndexed
            val dim = layer.dimBehind.coerceIn(0f, 1f)
            if (dim > 0f) steps += Step.Dim(dim)
            steps += Step.Draw(i, dx, dy)
        }
        return steps
    }
}
