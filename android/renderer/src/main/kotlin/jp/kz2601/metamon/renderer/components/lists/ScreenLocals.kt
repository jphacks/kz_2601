package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.staticCompositionLocalOf

/**
 * 画面ローカルの値(searchBar の検索文字列・iOS の `ScreenLocals`)。記録には保存せず、画面を離れると消える。
 * 同じ画面の `list.searchField` がこの値で絞り込む。
 */
class ScreenLocals {
    val values = mutableStateMapOf<String, String>()
}

/** 今の画面のローカル値。画面の外(プレビュー・適合テスト)は null。 */
val LocalScreenLocals = staticCompositionLocalOf<ScreenLocals?> { null }
