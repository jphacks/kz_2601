package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.renderer.components.actions.ButtonBusy
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** button の実行中と二度押しの防止(#1089・iOS の ButtonLeaf.showsProgress / begin)。 */
class ButtonBusyTest {
    private val lookup = Action(type = ActionType.LOOKUP_BOOK)
    private val save = Action(type = ActionType.SAVE_RECORD)

    @Test
    fun onlyWaitingActionsShowProgress() {
        assertTrue(ButtonBusy.waits(lookup))
        assertFalse(ButtonBusy.waits(save))
        assertFalse(ButtonBusy.waits(null))
    }

    @Test
    fun waitingActionBlocksRepeatTapUntilEnd() {
        val busy = ButtonBusy()
        assertTrue(busy.begin(lookup))
        assertTrue("押した瞬間に実行中", busy.running)
        assertFalse("実行中の二度押しは受けない", busy.begin(lookup))
        assertFalse("実行中は別のアクションも受けない", busy.begin(save))
        busy.end()
        assertFalse(busy.running)
        assertTrue("終わったらまた押せる", busy.begin(lookup))
    }

    @Test
    fun instantActionDoesNotSpin() {
        val busy = ButtonBusy()
        assertTrue(busy.begin(save))
        assertFalse("待たないアクションはくるくるを出さない", busy.running)
        assertTrue(busy.begin(save))
    }
}
