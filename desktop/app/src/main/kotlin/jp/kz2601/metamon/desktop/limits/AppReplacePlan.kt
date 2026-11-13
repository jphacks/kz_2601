package jp.kz2601.metamon.desktop.limits

import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ErrorCodes
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import java.time.Instant
import java.time.OffsetDateTime

/**
 * 402 `app_limit_reached` の中身(#630・iOS の `AppLimit`)。デスクトップでは枠を買えないので、コインの欄は読まない。
 * [apps] が null = サーバーが持っているアプリを返さなかった(端末の一覧を使う)。
 */
data class AppLimit(
    val held: Int,
    val limit: Int,
    val apps: List<HeldApp>? = null,
) {
    data class HeldApp(val id: String, val name: String = "", val mine: Boolean = true, val updatedAt: Instant? = null)

    companion object {
        /** code だけで見る(生成の stream の `event: error` は status が欠けるため・iOS と同じ)。 */
        fun from(error: Throwable): AppLimit? {
            val api = error as? ApiError.Server ?: return null
            if (api.code != ErrorCodes.APP_LIMIT_REACHED) return null
            val apps = (api.detail("apps") as? JsonArray)?.mapNotNull { item ->
                val o = item as? JsonObject ?: return@mapNotNull null
                fun str(key: String) = (o[key] as? JsonPrimitive)?.contentOrNull
                val id = str("id")?.takeIf { it.isNotEmpty() } ?: return@mapNotNull null
                HeldApp(
                    id = id,
                    name = str("name").orEmpty(),
                    mine = (o["mine"] as? JsonPrimitive)?.booleanOrNull ?: true,
                    updatedAt = str("updatedAt")?.let { runCatching { OffsetDateTime.parse(it).toInstant() }.getOrNull() },
                )
            }
            return AppLimit(
                held = api.detailNumber("held")?.toInt() ?: 0,
                limit = api.detailNumber("limit")?.toInt() ?: 0,
                apps = apps,
            )
        }
    }
}

/**
 * 「上限に達しています。どのアプリと置き換えますか?」に並べる候補(iOS の `AppReplacePlan` の純 Kotlin 版)。
 * 長く開いていない順・使用中のアプリは最後。自分で作った → 削除、参加した共有 → 抜ける。枠を買う道は出さない(デスクトップでは買えない)。
 */
data class AppReplacePlan(val rows: List<Row>, val held: Int, val limit: Int) {
    data class Row(
        val id: String,
        val name: String,
        val role: Role,
        /** 端末で最後に開いた時刻(無ければサーバーの更新日時) */
        val lastOpened: Instant?,
        val isCurrent: Boolean,
    )

    enum class Role {
        /** 自分で作った → 削除(データも消える) */
        OWN,

        /** 参加した共有のアプリ → 抜ける */
        JOINED,
    }

    sealed interface Action {
        data class Delete(val appId: String) : Action
        data class Leave(val appId: String) : Action
    }

    /** あと何個消せば 1 つ入るか(上限を下げた後は 2 つ以上のこともある) */
    val removalsNeeded: Int get() = maxOf(1, held - limit + 1)

    /** [id] を消した / 抜けた後の計画。null = もう 1 つ入る(元の操作を続ける)。 */
    fun removing(id: String): AppReplacePlan? {
        if (rows.none { it.id == id }) return this
        val next = AppReplacePlan(rows.filter { it.id != id }, maxOf(0, held - 1), limit)
        return if (next.held < next.limit) null else next
    }

    companion object {
        private val order: Comparator<Row> = compareBy<Row> { it.isCurrent }
            .thenBy { it.lastOpened != null }
            .thenBy { it.lastOpened }
            .thenBy { it.id }

        fun make(limit: AppLimit, local: List<AppSummary>, lastOpened: (String) -> Instant?, current: String?): AppReplacePlan {
            val localById = local.associateBy { it.id }
            val held = limit.apps
            val rows = if (!held.isNullOrEmpty()) {
                held.map { app ->
                    Row(
                        id = app.id,
                        name = app.name.ifEmpty { localById[app.id]?.name.orEmpty() },
                        role = if (app.mine) Role.OWN else Role.JOINED,
                        lastOpened = lastOpened(app.id) ?: app.updatedAt ?: localById[app.id]?.updatedInstant,
                        isCurrent = app.id == current,
                    )
                }
            } else {
                // 端末の一覧(空のアプリ・アーカイブは数えない)。持ち主は分からないので削除として扱う
                // (`DELETE /v1/apps/{id}` は参加したアプリなら自分の写しだけを消す)
                local.filter { !it.isArchived && !it.isBlank }.distinctBy { it.id }.map { app ->
                    Row(app.id, app.name, Role.OWN, lastOpened(app.id) ?: app.updatedInstant, app.id == current)
                }
            }
            return AppReplacePlan(rows.sortedWith(order), limit.held, limit.limit)
        }

        fun action(row: Row): Action = when (row.role) {
            Role.OWN -> Action.Delete(row.id)
            Role.JOINED -> Action.Leave(row.id)
        }
    }
}
