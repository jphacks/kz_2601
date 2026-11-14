package jp.kz2601.metamon.renderer.components.media

import kotlin.math.ceil
import kotlin.math.max
import kotlin.math.min

/** 線の 1 点(キャンバスの px)。 */
data class InkPoint(val x: Float, val y: Float)

/** 1 本の線。[width] はキャンバスの px。 */
data class InkStroke(val points: List<InkPoint>, val width: Float)

/**
 * 描いている間の状態(iOS の `DrawingSession`)。変えるたびに新しい値を返す(Compose の状態にそのまま置ける)。
 * 消しゴムは線ごと消す(PencilKit の既定と同じ)。元に戻す・やり直すは線を足す・消す・全部消すの単位。
 */
data class DrawingState(
    val strokes: List<InkStroke> = emptyList(),
    /** 保存済みの絵(描き足す時の下絵)があるか。「全部消す」で消える */
    val hasBackground: Boolean = false,
    private val undoStack: List<Snapshot> = emptyList(),
    private val redoStack: List<Snapshot> = emptyList(),
) {
    data class Snapshot(val strokes: List<InkStroke>, val hasBackground: Boolean)

    val isEmpty: Boolean get() = strokes.isEmpty() && !hasBackground
    val canUndo: Boolean get() = undoStack.isNotEmpty()
    val canRedo: Boolean get() = redoStack.isNotEmpty()

    /** 開いた時から何か変えたか(閉じる時に「保存せずに閉じますか?」を出すか)。 */
    val isChanged: Boolean get() = undoStack.isNotEmpty()

    private val snapshot get() = Snapshot(strokes, hasBackground)

    /** 元に戻せる 1 手を積んでから変える。 */
    private fun edit(strokes: List<InkStroke> = this.strokes, hasBackground: Boolean = this.hasBackground) =
        copy(strokes = strokes, hasBackground = hasBackground, undoStack = undoStack + snapshot, redoStack = emptyList())

    fun adding(stroke: InkStroke): DrawingState = if (stroke.points.isEmpty()) this else edit(strokes = strokes + stroke)

    /** [at] から [radius] 以内に点がある線を消す。消えなければ変えない(元に戻すの 1 手も積まない)。 */
    fun erasing(at: InkPoint, radius: Float): DrawingState {
        val kept = strokes.filterNot { s -> s.points.any { p -> hit(p, at, radius + s.width / 2) } }
        return if (kept.size == strokes.size) this else edit(strokes = kept)
    }

    /** 全部消す: 線も下絵も消す。 */
    fun cleared(): DrawingState = if (isEmpty) this else edit(strokes = emptyList(), hasBackground = false)

    fun undone(): DrawingState {
        val prev = undoStack.lastOrNull() ?: return this
        return copy(strokes = prev.strokes, hasBackground = prev.hasBackground, undoStack = undoStack.dropLast(1), redoStack = redoStack + snapshot)
    }

    fun redone(): DrawingState {
        val next = redoStack.lastOrNull() ?: return this
        return copy(strokes = next.strokes, hasBackground = next.hasBackground, undoStack = undoStack + snapshot, redoStack = redoStack.dropLast(1))
    }

    private fun hit(p: InkPoint, at: InkPoint, r: Float): Boolean {
        val dx = p.x - at.x
        val dy = p.y - at.y
        return dx * dx + dy * dy <= r * r
    }
}

/** 手書きを photo field の値にする時の決まり(iOS の `DrawingCodec`)。白地・長い辺 800px 以下・JPEG 品質 70。 */
object DrawingCodec {
    const val MAX_SIDE = 800
    const val QUALITY = 70

    /** 長い辺が [MAX_SIDE] を少し超える倍率(1〜3)で描き、写真と同じ縮小で 800px にする(線がにじまない)。 */
    fun renderScale(canvasWidth: Float, canvasHeight: Float): Float =
        min(3f, max(1f, ceil(MAX_SIDE / max(canvasWidth, canvasHeight))))

    /** 下絵をキャンバスに収める位置(aspect fit・中央)。[left, top, width, height]。 */
    fun aspectFit(imageWidth: Float, imageHeight: Float, canvasWidth: Float, canvasHeight: Float): FloatArray {
        if (imageWidth <= 0f || imageHeight <= 0f) return floatArrayOf(0f, 0f, canvasWidth, canvasHeight)
        val r = min(canvasWidth / imageWidth, canvasHeight / imageHeight)
        val w = imageWidth * r
        val h = imageHeight * r
        return floatArrayOf((canvasWidth - w) / 2, (canvasHeight - h) / 2, w, h)
    }
}
