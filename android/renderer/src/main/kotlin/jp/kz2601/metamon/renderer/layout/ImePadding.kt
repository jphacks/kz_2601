package jp.kz2601.metamon.renderer.layout

import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.composed
import androidx.compose.ui.layout.boundsInRoot
import androidx.compose.ui.layout.findRootCoordinates
import androidx.compose.ui.layout.onGloballyPositioned

/**
 * キーボードの分だけ下を空ける。ただし自分の下にもうある分(タブバー・Scaffold の下の帯など)は引く。
 * imePadding はキーボードの高さをまるごと足すので、下に帯があるとその分だけキーボードの上に余計に浮く。
 */
fun Modifier.imePaddingExcludingBelow(): Modifier = composed {
    var below by remember { mutableIntStateOf(0) }
    onGloballyPositioned { c -> below = distanceBelow(c.findRootCoordinates().size.height, c.boundsInRoot().bottom) }
        .windowInsetsPadding(imeExcludingBelow(WindowInsets.ime, below))
}

/** 自分の下端から窓の下端までの px。 */
internal fun distanceBelow(rootHeight: Int, bottom: Float): Int = (rootHeight - bottom).toInt().coerceAtLeast(0)

/** キーボードの高さから下にある分を引いた insets(0 未満にはならない)。 */
internal fun imeExcludingBelow(ime: WindowInsets, belowPx: Int): WindowInsets = ime.exclude(WindowInsets(bottom = belowPx))
