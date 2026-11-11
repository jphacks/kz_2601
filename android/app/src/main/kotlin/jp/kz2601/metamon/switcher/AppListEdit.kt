package jp.kz2601.metamon.switcher

import jp.kz2601.metamon.services.AppSummary

/**
 * 変身一覧の整理の純粋な部分(iOS の `AppListEdit` / `AppDeletion.fallback`・#277 / #180)。
 * 名前と順番の正典はサーバー(`PATCH /v1/apps/{appId}`・`PUT /v1/apps/order`)。
 */
object AppListEdit {
    /** サーバー(appconfig `maxAppNameRunes`)と同じ上限。 */
    const val MAX_NAME_LENGTH = 40

    /** 前後の空白を除いた名前。空・長すぎは null。 */
    fun normalizedName(raw: String): String? {
        val name = raw.trim()
        if (name.isEmpty() || name.runes() > MAX_NAME_LENGTH) return null
        return name
    }

    /** 複製の既定の名前。[suffixed](「○○のコピー」)が上限を超えるなら元の名前を後ろから削る。 */
    fun copyName(of: String, suffixed: (String) -> String): String {
        var base = of
        while (suffixed(base).runes() > MAX_NAME_LENGTH && base.isNotEmpty()) {
            base = base.substring(0, base.offsetByCodePoints(base.length, -1))
        }
        return suffixed(base)
    }

    /** [ids] の中で [id] を [index] に動かした順番。無ければそのまま・範囲外は端に寄せる。 */
    fun moved(ids: List<String>, id: String, index: Int): List<String> {
        val from = ids.indexOf(id)
        if (from < 0) return ids
        val out = ids.toMutableList()
        out.removeAt(from)
        out.add(index.coerceIn(0, out.size), id)
        return out
    }

    /** [ids] の順に sortIndex = 位置 + 1 を振る。[ids] に無いアプリはそのまま(サーバーの `PUT /v1/apps/order` と同じ)。 */
    fun applyingOrder(ids: List<String>, apps: List<AppSummary>): List<AppSummary> {
        val position = HashMap<String, Int>()
        ids.forEachIndexed { i, id -> position.putIfAbsent(id, i + 1) }
        return apps.map { app -> position[app.id]?.let { app.copy(sortIndex = it) } ?: app }
    }

    /** 使用中のアプリを消した後の行き先。 */
    sealed interface Fallback {
        /** 残りのうち一覧の先頭のアプリ */
        data class App(val id: String) : Fallback

        /** 残りが無ければ話しかける前の画面 */
        data object Home : Fallback
    }

    /** [deleting] を消した後の切り替え先。使用中のアプリでなければ null(切り替えない)。 */
    fun fallback(deleting: String, current: String?, apps: List<AppSummary>): Fallback? {
        if (deleting != current) return null
        val rest = AppSummary.visible(apps.filter { it.id != deleting }, current = null)
        return rest.firstOrNull()?.let { Fallback.App(it.id) } ?: Fallback.Home
    }

    private fun String.runes(): Int = codePointCount(0, length)
}
