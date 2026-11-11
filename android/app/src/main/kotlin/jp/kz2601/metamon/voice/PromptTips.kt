package jp.kz2601.metamon.voice

import androidx.annotation.StringRes
import jp.kz2601.metamon.R

/**
 * 「頼み方のコツ」(iOS の `PromptTips`・#415)。タブ・画面の行き来・一覧などを正確に頼める言い方をカテゴリ別に並べる。
 * 文は iOS と同じ(ai-eval の `tips_cases.json` で確かめている文)。例をタップすると入力欄に入る(送らない)。
 */
object PromptTips {
    /** 例が使う機能のうち、版によって無いもの */
    enum class Feature { SHARING, CAMERA_CAPTURE, NEARBY, HEALTH, LOCATION_TRACKING }

    data class Tip(
        val id: String,
        @StringRes val text: Int,
        val requires: Set<Feature> = emptySet(),
        /** AI に送らず端末で処理する(「元に戻して」) */
        val onDevice: Boolean = false,
    )

    data class Category(val id: String, @StringRes val title: Int, val tips: List<Tip>)

    /** 版で使えない例を除いたカテゴリ。例が 0 になったカテゴリは出さない */
    fun categories(available: Set<Feature> = Feature.entries.toSet()): List<Category> = all.mapNotNull { c ->
        val tips = c.tips.filter { available.containsAll(it.requires) }
        if (tips.isEmpty()) null else c.copy(tips = tips)
    }

    val all: List<Category> = listOf(
        Category("tabs", R.string.voice_tips_cat_tabs, listOf(
            Tip("tab_add", R.string.voice_tip_tab_add),
            Tip("tab_rename", R.string.voice_tip_tab_rename),
            Tip("tab_reorder", R.string.voice_tip_tab_reorder),
        )),
        Category("nav", R.string.voice_tips_cat_nav, listOf(
            Tip("nav_button", R.string.voice_tip_nav_button),
            Tip("nav_new_screen", R.string.voice_tip_nav_new_screen),
            Tip("nav_back", R.string.voice_tip_nav_back),
        )),
        Category("detail", R.string.voice_tips_cat_detail, listOf(
            Tip("detail_open", R.string.voice_tip_detail_open),
            Tip("detail_edit", R.string.voice_tip_detail_edit),
            Tip("detail_swipe", R.string.voice_tip_detail_swipe),
        )),
        Category("add", R.string.voice_tips_cat_add, listOf(
            Tip("add_button", R.string.voice_tip_add_button),
            Tip("add_field", R.string.voice_tip_add_field),
            Tip("add_move", R.string.voice_tip_add_move),
        )),
        Category("stats", R.string.voice_tips_cat_stats, listOf(
            Tip("stats_total", R.string.voice_tip_stats_total),
            Tip("stats_pie", R.string.voice_tip_stats_pie),
            Tip("stats_line", R.string.voice_tip_stats_line),
            Tip("stats_steps", R.string.voice_tip_stats_steps, setOf(Feature.HEALTH)),
        )),
        Category("notify", R.string.voice_tips_cat_notify, listOf(
            Tip("notify_daily", R.string.voice_tip_notify_daily),
            Tip("notify_weekly", R.string.voice_tip_notify_weekly),
            Tip("notify_change", R.string.voice_tip_notify_change),
        )),
        Category("share", R.string.voice_tips_cat_share, listOf(
            Tip("share_family", R.string.voice_tip_share_family, setOf(Feature.SHARING)),
            Tip("share_public", R.string.voice_tip_share_public, setOf(Feature.SHARING)),
            Tip("share_nearby", R.string.voice_tip_share_nearby, setOf(Feature.SHARING, Feature.NEARBY)),
        )),
        Category("map", R.string.voice_tips_cat_map, listOf(
            Tip("map_pins", R.string.voice_tip_map_pins),
            Tip("map_here", R.string.voice_tip_map_here),
            Tip("map_route", R.string.voice_tip_map_route, setOf(Feature.LOCATION_TRACKING)),
        )),
        Category("photo", R.string.voice_tips_cat_photo, listOf(
            Tip("photo_attach", R.string.voice_tip_photo_attach),
            Tip("photo_camera", R.string.voice_tip_photo_camera, setOf(Feature.CAMERA_CAPTURE)),
            Tip("photo_qr", R.string.voice_tip_photo_qr),
        )),
        Category("fix", R.string.voice_tips_cat_fix, listOf(
            Tip("fix_bigger", R.string.voice_tip_fix_bigger),
            Tip("fix_color", R.string.voice_tip_fix_color),
            Tip("fix_words", R.string.voice_tip_fix_words),
            Tip("fix_undo", R.string.voice_tip_fix_undo, onDevice = true),
        )),
    )
}
