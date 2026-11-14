package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.DebugKnobs
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.navigation.top

/**
 * `METAMON_AUTO_NAV`(#1024・iOS の `AppRuntime+AutoNav`): 今見えている画面から navigate のアクションを実行する。
 * 本物の遷移(#357)を通るので、`:sheet` / `:fullScreen` の出し方も、タブの切り替え・push も画面と同じに動く。
 */
object AutoNav {
    fun action(step: DebugKnobs.AutoNavStep): Action = Action(
        type = ActionType.NAVIGATE,
        screen = step.screen,
        presentation = step.presentation?.let { p -> Presentation.entries.firstOrNull { it.wire == p } },
    )

    /** 1 歩進めて、ログの行(スクショ側が同期に使う)を返す。 */
    suspend fun step(runtime: AppRuntime, step: DebugKnobs.AutoNavStep, index: Int, total: Int): String {
        // DSL に無い画面へは行かない(空の画面を積まない)
        val found = runtime.dsl.screens.any { it.id == step.screen }
        if (found) runtime.run(action(step), ActionContext(route = runtime.navigation.value.top(runtime.dsl)))
        val nav = runtime.navigation.value
        val to = nav.top(runtime.dsl)
        return "auto nav ${index + 1}/$total: → $step ${if (found) "ok" else "not found"} " +
            "tab=${nav.selectedTab.ifEmpty { "-" }} stack=${to.stack} depth=${nav.path(to.stack).size} " +
            "sheet=${nav.sheet?.screen ?: "-"} full=${nav.fullScreen?.screen ?: "-"}"
    }
}
