package jp.kz2601.metamon.renderer.components.scan

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** カメラで読む Leaf(#866)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerScanLeaves() {
    register(ComponentType.QR_SCANNER) { c, ctx, m -> QrScannerLeaf(c, ctx, m) }
}
