package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component

/**
 * 編集(2 回目以降の変身)で変わった部品(#894・iOS の `ChangeHighlight` と `BuildChoreography.editHighlight`)。
 * 描画器は画面の出現時に、変わった部品だけを光らせて置き直す。
 */
data class ChangeHighlight(
    /** [key] の集合 */
    val components: Set<String> = emptySet(),
    /** 変わった部品を持つ画面 id。ここに無い画面はいつもどおり */
    val screens: Set<String> = emptySet(),
) {
    val isEmpty: Boolean get() = components.isEmpty()

    fun contains(screen: String, component: Component, index: Int): Boolean = key(screen, component, index) in components

    companion object {
        val NONE = ChangeHighlight()

        /**
         * `画面 id/部品 id`。部品 id の無い DSL もあるので、無ければ画面直下の位置(`#2`)で代わりにする
         * (iOS は読み込み時に UUID を振るが、Android は id を持たないまま読む)。
         */
        fun key(screen: String, component: Component, index: Int): String = screen + "/" + (component.id ?: "#$index")

        /** 新しい画面・ほぼ全部が変わった画面は光らせない(普段の出現に任せる)境目 */
        const val MAX_CHANGED_RATIO = 0.7

        /**
         * 編集の前後で、変わった(足された・中身が変わった・場所が変わった)画面直下の部品。
         * 部品 id は付け直されることがあるので、id ではなく中身の印([signature])で比べる。
         */
        fun between(old: AppDSL, new: AppDSL): ChangeHighlight {
            val keys = mutableSetOf<String>()
            val screens = mutableSetOf<String>()
            for (screen in new.screens) {
                val before = old.screens.firstOrNull { it.id == screen.id } ?: continue
                if (screen.components.isEmpty()) continue
                val remaining = before.components.map(::signature).toMutableList()
                val changed = mutableListOf<String>()
                screen.components.forEachIndexed { i, c ->
                    if (!remaining.remove(signature(c))) changed += key(screen.id, c, i)
                }
                if (changed.isEmpty() || changed.size.toDouble() / screen.components.size > MAX_CHANGED_RATIO) continue
                screens += screen.id
                keys += changed
            }
            return ChangeHighlight(keys, screens)
        }

        /** 部品の中身の印(見た目に効く項目と子の印)。 */
        fun signature(c: Component): String {
            val fields = listOf(
                c.rawType.ifEmpty { c.type.wire }, c.area, c.text, c.label, c.title, c.icon, c.style?.wire, c.field, c.entity,
                c.kind?.wire, c.color?.wire, c.placeholder, c.aggregate?.wire, c.format?.wire, c.groupBy, c.align?.wire, c.size?.wire,
            )
            val own = fields.joinToString("\u001F") { it ?: "" }
            return own + "[" + c.children.joinToString("\u001E", transform = ::signature) + "]"
        }
    }
}

/** タブ表示時の初期タブ(= tab.screen)。initialScreen がタブの根ならそれ、違えば先頭タブ。タブ無しなら空文字(iOS の `initialTab`)。 */
val AppDSL.initialTab: String
    get() = app.tabs.firstOrNull { it.screen == initialScreen }?.screen ?: app.tabs.firstOrNull()?.screen ?: ""
