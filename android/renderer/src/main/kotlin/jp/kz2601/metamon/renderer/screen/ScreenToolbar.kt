package jp.kz2601.metamon.renderer.screen

import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.ToolbarButton
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.isButtonVisible
import jp.kz2601.metamon.renderer.form.FormDraft

/**
 * 見出しの toolbar と画面下の保存バーに何を出すか(iOS の `DuplicateAddControl.toolbar`・`FormSaveBar.pinned` / `hasSave`)。
 * 画面と適合テストの木が同じ関数を使う。
 */
object ScreenToolbar {
    private data class Opener(val screen: String, val prefillRef: String?)

    private val Screen.allComponents: List<Component> get() = components.flatMap { listOf(it) + it.children }

    /** 同じフォーム画面を同じ prefillRef で開く button があれば、toolbar の方は出さない(#416)。 */
    fun deduplicated(screen: Screen, dsl: AppDSL): List<ToolbarButton> {
        val buttons = screen.allComponents.mapNotNull { c ->
            val a = c.action
            if (c.type != ComponentType.BUTTON || a == null || a.type != ActionType.NAVIGATE) return@mapNotNull null
            a.screen?.let { Opener(it, a.prefillRef) }
        }.toSet()
        if (buttons.isEmpty()) return screen.toolbar
        return screen.toolbar.filter { tb ->
            val a = tb.action
            val target = a.screen
            if (a.type != ActionType.NAVIGATE || target == null || dsl.screen(target)?.entity == null) true
            else Opener(target, a.prefillRef) !in buttons
        }
    }

    /** フォーム画面の saveRecord は画面下の保存バーに移す(見出しからは外す)。 */
    fun pinned(screen: Screen): List<ToolbarButton> =
        if (screen.entity == null) emptyList() else screen.toolbar.filter { it.action.type == ActionType.SAVE_RECORD }

    /** 画面のどこかに saveRecord がある(= 編集フォーム)。 */
    fun hasSave(screen: Screen): Boolean =
        screen.toolbar.any { it.action.type == ActionType.SAVE_RECORD } ||
            screen.allComponents.any { it.action?.type == ActionType.SAVE_RECORD }

    /** 見出しに出す項目(重複した追加・保存バーに移した項目・見えないボタンを除く)。[dsl] が無ければ重複は外さない。 */
    fun header(screen: Screen, dsl: AppDSL?, form: FormDraft?): List<ToolbarButton> {
        val pinned = pinned(screen)
        return (dsl?.let { deduplicated(screen, it) } ?: screen.toolbar).filter { tb -> pinned.none { it === tb } && isButtonVisible(tb.action, form) }
    }

    /** 画面下の保存バー。 */
    sealed interface SaveBar {
        /** 他人の共有レコード: 「作った人だけが変更できます」 */
        data object ReadOnly : SaveBar
        data class Save(val button: ToolbarButton) : SaveBar
    }

    fun saveBar(screen: Screen, form: FormDraft?): SaveBar? = when {
        form?.isReadOnly == true && hasSave(screen) -> SaveBar.ReadOnly
        else -> pinned(screen).firstOrNull()?.let(SaveBar::Save)
    }
}
