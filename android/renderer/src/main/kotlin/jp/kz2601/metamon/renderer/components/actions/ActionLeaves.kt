package jp.kz2601.metamon.renderer.components.actions

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/**
 * 押すと外へ出る / 外のものを見せる Leaf(#861)。1 行 1 部品・名前順。button は `Leaves.kt`(#357)で登録済み。
 * 見た目のボタンは [ActionButton]、外へ開くのは [openExternal] / [shareText]。
 */
fun ComponentRegistry.Builder.registerActionLeaves() {
    register(ComponentType.OPEN_URL_BUTTON) { c, ctx, m -> OpenUrlButtonLeaf(c, ctx, m) }
    register(ComponentType.QR_CODE) { c, ctx, m -> QrCodeLeaf(c, ctx, m) }
    register(ComponentType.SHARE_BUTTON) { c, ctx, m -> ShareButtonLeaf(c, ctx, m) }
    register(ComponentType.WEB_VIEW) { c, _, m -> WebViewLeaf(c, m) }
}
