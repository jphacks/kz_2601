package jp.kz2601.metamon.renderer.components.importing

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** ファイルの取り込み(#865)。1 行 1 部品・名前順。流れは [FileImportFlow]、記録を足す口は [RecordImportHost]。 */
fun ComponentRegistry.Builder.registerImportLeaves() {
    register(ComponentType.FILE_IMPORT) { c, ctx, m -> FileImportLeaf(c, ctx, m) }
}
