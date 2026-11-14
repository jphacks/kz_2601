package jp.kz2601.metamon.renderer.components.music

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry

/** 音楽の Leaf(#879)。1 行 1 部品・名前順。 */
fun ComponentRegistry.Builder.registerMusicLeaves() {
    register(ComponentType.MUSIC_PLAYER) { c, _, m -> MusicPlayerLeaf(c, m) }
}
