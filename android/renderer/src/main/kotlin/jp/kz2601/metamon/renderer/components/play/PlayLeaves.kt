package jp.kz2601.metamon.renderer.components.play

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 計算・遊び系の Leaf 5 種(#860)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerPlayLeaves() {
    register(ComponentType.BRACKET) { c, ctx, m -> BracketLeaf(c, ctx, m) }
    register(ComponentType.FLASHCARDS) { c, ctx, m -> FlashcardsLeaf(c, ctx, m) }
    register(ComponentType.RANDOM_PICKER) { c, ctx, m -> RandomPickerLeaf(c, ctx, m) }
    register(ComponentType.SETTLEMENT) { c, ctx, m -> SettlementLeaf(c, ctx, m) }
    register(ComponentType.TIMER_DISPLAY) { c, ctx, m -> TimerDisplayLeaf(c, ctx, m) }
}
