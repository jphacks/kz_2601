package jp.kz2601.metamon.renderer.screen

import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.geometry.Rect

/**
 * ホスト(マイク FAB)と描画器のやり取り(iOS の `GridBottomObstructionKey` / `fabFrame`)。
 * 描画器は帯に掛かる部品の上端を [report] し、ホストは FAB の矩形を [fabFrame] に入れる。座標はルート座標(px)。
 */
class FabCoordinator {
    /** 止まっている FAB の矩形。隠れている・出していない・動かしている間は null */
    var fabFrame: Rect? by mutableStateOf(null)

    private val leading = mutableStateMapOf<String, Float>()
    private val trailing = mutableStateMapOf<String, Float>()

    /** 画面 [screenId] の帯に掛かる部品の上端(無ければ null)。画面が消えたら [forget] */
    fun report(screenId: String, leadingTop: Float?, trailingTop: Float?) {
        if (leadingTop == null) leading.remove(screenId) else leading[screenId] = leadingTop
        if (trailingTop == null) trailing.remove(screenId) else trailing[screenId] = trailingTop
    }

    fun forget(screenId: String) = report(screenId, null, null)

    /** FAB を寄せた側の、いちばん高い部品の上端(無ければ null) */
    fun obstructionTop(leadingSide: Boolean): Float? = (if (leadingSide) leading else trailing).values.minOrNull()
}

/** ホストが FAB を重ねている時だけ入る。null = FAB が無い(余白も退避もしない) */
val LocalFabCoordinator = compositionLocalOf<FabCoordinator?> { null }
