package jp.kz2601.metamon.desktop.voice

/**
 * 「頼み方のコツ」(Android `voice/PromptTips.kt`・iOS `PromptTips`・#415)のデスクトップ版の中身。
 * Android は `@StringRes` で取り込めないので、カテゴリ・例の id・文言のキーだけを写す。文言は Android の strings を同じキーで引く。
 * デスクトップに無い機能(撮影・近くの人・ヘルスケア・位置の記録)の例は出さない。
 */
object PromptTips {
    /** 例が使う機能 */
    enum class Feature { SHARING, CAMERA_CAPTURE, NEARBY, HEALTH, LOCATION_TRACKING }

    data class Tip(
        val id: String,
        val textKey: String,
        val requires: Set<Feature> = emptySet(),
        /** AI に送らず端末で処理する(「元に戻して」) */
        val onDevice: Boolean = false,
    )

    data class Category(val id: String, val titleKey: String, val tips: List<Tip>)

    /** デスクトップで使える機能 */
    val available: Set<Feature> = setOf(Feature.SHARING)

    /** 使えない例を除いたカテゴリ。例が 0 になったカテゴリは出さない */
    fun categories(available: Set<Feature> = PromptTips.available): List<Category> = all.mapNotNull { c ->
        val tips = c.tips.filter { available.containsAll(it.requires) }
        if (tips.isEmpty()) null else c.copy(tips = tips)
    }

    val all: List<Category> = listOf(
        Category("tabs", "voice_tips_cat_tabs", listOf(
            Tip("tab_add", "voice_tip_tab_add"),
            Tip("tab_rename", "voice_tip_tab_rename"),
            Tip("tab_reorder", "voice_tip_tab_reorder"),
        )),
        Category("nav", "voice_tips_cat_nav", listOf(
            Tip("nav_button", "voice_tip_nav_button"),
            Tip("nav_new_screen", "voice_tip_nav_new_screen"),
            Tip("nav_back", "voice_tip_nav_back"),
        )),
        Category("detail", "voice_tips_cat_detail", listOf(
            Tip("detail_open", "voice_tip_detail_open"),
            Tip("detail_edit", "voice_tip_detail_edit"),
            Tip("detail_swipe", "voice_tip_detail_swipe"),
        )),
        Category("add", "voice_tips_cat_add", listOf(
            Tip("add_button", "voice_tip_add_button"),
            Tip("add_field", "voice_tip_add_field"),
            Tip("add_move", "voice_tip_add_move"),
        )),
        Category("stats", "voice_tips_cat_stats", listOf(
            Tip("stats_total", "voice_tip_stats_total"),
            Tip("stats_pie", "voice_tip_stats_pie"),
            Tip("stats_line", "voice_tip_stats_line"),
            Tip("stats_steps", "voice_tip_stats_steps", setOf(Feature.HEALTH)),
        )),
        Category("notify", "voice_tips_cat_notify", listOf(
            Tip("notify_daily", "voice_tip_notify_daily"),
            Tip("notify_weekly", "voice_tip_notify_weekly"),
            Tip("notify_change", "voice_tip_notify_change"),
        )),
        Category("share", "voice_tips_cat_share", listOf(
            Tip("share_family", "voice_tip_share_family", setOf(Feature.SHARING)),
            Tip("share_public", "voice_tip_share_public", setOf(Feature.SHARING)),
            Tip("share_nearby", "voice_tip_share_nearby", setOf(Feature.SHARING, Feature.NEARBY)),
        )),
        Category("map", "voice_tips_cat_map", listOf(
            Tip("map_pins", "voice_tip_map_pins"),
            Tip("map_here", "voice_tip_map_here"),
            Tip("map_route", "voice_tip_map_route", setOf(Feature.LOCATION_TRACKING)),
        )),
        Category("photo", "voice_tips_cat_photo", listOf(
            Tip("photo_attach", "voice_tip_photo_attach"),
            Tip("photo_camera", "voice_tip_photo_camera", setOf(Feature.CAMERA_CAPTURE)),
            Tip("photo_qr", "voice_tip_photo_qr"),
        )),
        Category("fix", "voice_tips_cat_fix", listOf(
            Tip("fix_bigger", "voice_tip_fix_bigger"),
            Tip("fix_color", "voice_tip_fix_color"),
            Tip("fix_words", "voice_tip_fix_words"),
            Tip("fix_undo", "voice_tip_fix_undo", onDevice = true),
        )),
    )
}
