package jp.kz2601.metamon.desktop

import androidx.compose.ui.InternalComposeUiApi
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEvent
import androidx.compose.ui.input.key.KeyEventType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 写真のプレビューの Space の押しっぱなし(isAutoRepeatSpace): 開いたり閉じたりを繰り返さないこと。 */
class PhotoPreviewSpaceRepeatTest {
    /** 出来事ごとに別の元の出来事(同じ物かは元の出来事で見分ける)。元の出来事を渡す口は Compose の内部向け。 */
    @OptIn(InternalComposeUiApi::class)
    private fun event(type: KeyEventType, key: Key = Key.Spacebar): KeyEvent = KeyEvent(key = key, type = type, nativeEvent = Any())

    private fun down() = event(KeyEventType.KeyDown)

    private fun up() = event(KeyEventType.KeyUp)

    @Before
    fun forget() = resetSpaceRepeats()

    @Test
    fun heldSpaceTogglesOnlyOnce() {
        // 押しっぱなしの出来事を「開く / 閉じる」に通し、切り替わった回数を数える
        var open = false
        var toggles = 0
        fun press(e: KeyEvent) {
            if (!e.isAutoRepeatSpace()) {
                open = !open
                toggles++
            }
        }
        press(down())
        repeat(10) { press(down()) }
        assertTrue(open)
        assertEquals(1, toggles)
        assertFalse(up().isAutoRepeatSpace())
        // 離してからもう一度押すと閉じる
        press(down())
        repeat(5) { press(down()) }
        assertFalse(open)
        assertEquals(2, toggles)
    }

    @Test
    fun sameEventSeenTwiceGivesSameAnswer() {
        // 部品と一番外が同じ出来事を見る
        val first = down()
        assertFalse(first.isAutoRepeatSpace())
        assertFalse(first.isAutoRepeatSpace())
        val second = down()
        assertTrue(second.isAutoRepeatSpace())
        assertTrue(second.isAutoRepeatSpace())
    }

    @Test
    fun otherKeysAndTypedEventsAreNeverRepeats() {
        assertFalse(event(KeyEventType.KeyDown, Key.Enter).isAutoRepeatSpace())
        assertFalse(event(KeyEventType.KeyDown, Key.Enter).isAutoRepeatSpace())
        // 種類の分からない出来事(文字の出来事など)は押しっぱなしの数に入れず、押した状態も崩さない
        assertFalse(down().isAutoRepeatSpace())
        assertFalse(event(KeyEventType.Unknown).isAutoRepeatSpace())
        assertTrue(down().isAutoRepeatSpace())
    }

    @Test
    fun resetAfterLostKeyUpTreatsNextPressAsFirst() {
        assertFalse(down().isAutoRepeatSpace())
        assertTrue(down().isAutoRepeatSpace())
        // ウィンドウの外で離した(離した出来事が届かない)
        resetSpaceRepeats()
        assertFalse(down().isAutoRepeatSpace())
    }
}
