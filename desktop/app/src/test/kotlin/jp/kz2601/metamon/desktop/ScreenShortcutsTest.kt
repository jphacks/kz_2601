package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.ToolbarButton
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.form.FormDraft
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Test

/** Ctrl+S・Delete が拾うボタン(ScreenShortcuts の saveAction・buttons・deleteAction)。 */
class ScreenShortcutsTest {
    private val entity = Entity(id = "notes", name = "メモ")
    private val save = Action(type = ActionType.SAVE_RECORD, entity = "other")
    private val delete = Action(type = ActionType.DELETE_RECORD, entity = "notes")

    private fun button(label: String, action: Action, children: List<Component> = emptyList()) =
        Component(type = ComponentType.BUTTON, label = label, action = action, children = children)

    private fun editing(canDelete: Boolean = true) =
        FormDraft(entity, Record(id = "r1")).also { it.canDelete = canDelete }

    @Test
    fun saveActionPrefersScreenButton() {
        val screen = Screen(id = "form", entity = "notes", components = listOf(button("保存", save)))
        assertSame(save, ScreenShortcuts.saveAction(screen, FormDraft(entity)))
    }

    @Test
    fun saveActionFallsBackToFormEntity() {
        val screen = Screen(id = "form", entity = "notes")
        val a = ScreenShortcuts.saveAction(screen, FormDraft(entity))
        assertEquals(ActionType.SAVE_RECORD, a?.type)
        assertEquals("notes", a?.entity)
    }

    @Test
    fun saveActionIsNullForReadOnlyFormOrNoForm() {
        val screen = Screen(id = "form", entity = "notes", components = listOf(button("保存", save)))
        assertNull(ScreenShortcuts.saveAction(screen, FormDraft(entity).also { it.isReadOnly = true }))
        assertNull("フォームも保存ボタンも無い", ScreenShortcuts.saveAction(Screen(id = "home"), null))
    }

    @Test
    fun saveActionWithoutFormUsesScreenButton() {
        val screen = Screen(id = "home", components = listOf(button("保存", save)))
        assertSame(save, ScreenShortcuts.saveAction(screen, null))
    }

    @Test
    fun buttonsListToolbarThenComponentsWithChildren() {
        val nav = Action(type = ActionType.NAVIGATE, screen = "a")
        val screen = Screen(
            id = "home",
            toolbar = listOf(ToolbarButton(label = "追加", icon = "plus", action = nav)),
            components = listOf(
                Component(type = ComponentType.TEXT, text = "見出し"),
                Component(type = ComponentType.SECTION, children = listOf(button("保存", save))),
                Component(type = ComponentType.BUTTON, title = "題名だけ", action = delete),
                Component(type = ComponentType.BUTTON, label = "アクション無し"),
            ),
        )
        val bs = ScreenShortcuts.buttons(screen)
        assertEquals(listOf("追加", "保存", "題名だけ"), bs.map { it.first })
        assertEquals(listOf("plus", null, null), bs.map { it.second })
        assertEquals(listOf(nav, save, delete), bs.map { it.third })
    }

    @Test
    fun deleteActionAddsConfirmWhenMissing() {
        val screen = Screen(id = "form", entity = "notes", components = listOf(button("削除", delete)))
        val a = ScreenShortcuts.deleteAction(screen, editing())
        assertEquals(KeyboardText.DELETE_CONFIRM, a?.confirmMessage)
    }

    @Test
    fun deleteActionKeepsOwnConfirm() {
        val own = delete.copy(confirmMessage = "本当に消す?")
        val screen = Screen(id = "form", entity = "notes", components = listOf(button("削除", own)))
        assertEquals("本当に消す?", ScreenShortcuts.deleteAction(screen, editing())?.confirmMessage)
    }

    @Test
    fun deleteActionHiddenForNewOrUndeletableForm() {
        val screen = Screen(id = "form", entity = "notes", components = listOf(button("削除", delete)))
        assertNull("新規フォーム", ScreenShortcuts.deleteAction(screen, FormDraft(entity).also { it.canDelete = true }))
        assertNull("消せない記録", ScreenShortcuts.deleteAction(screen, editing(canDelete = false)))
        assertNull("フォームが無い", ScreenShortcuts.deleteAction(screen, null))
    }
}
