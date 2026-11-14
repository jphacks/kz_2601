package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.io.IOException

/** サーバーのアプリの一覧を読んだ結果(#2378)。 */
sealed interface ServerAppsResult {
    /** [fromCache] は前回の写し(圏外・5xx で読めなかった)。 */
    data class Loaded(val apps: List<AppSummary>, val fromCache: Boolean) : ServerAppsResult

    /** サインインしていない(呼んでいない)。 */
    data object SignedOut : ServerAppsResult

    /** 読めず、写しも無い。または写しで代えない失敗(401・404 など)。 */
    data class Failed(val error: Exception) : ServerAppsResult
}

/**
 * サーバーのアプリの一覧(`GET /v1/apps`)。並びは自分で並べた順(`PUT /v1/apps/order` が付ける `sortIndex`)に合わせる(#2378)。
 * 読めたら写しを [cache] に置き、圏外・5xx では前回の写しを返す。サインインしていない時は呼ばない。
 */
class ServerApps(
    private val client: AppConfigClient?,
    private val cache: DocumentCache,
    private val isSignedIn: () -> Boolean,
) {
    suspend fun load(): ServerAppsResult {
        val c = client ?: return ServerAppsResult.SignedOut
        if (!isSignedIn()) return ServerAppsResult.SignedOut
        val epoch = cache.epoch
        val apps = try {
            ordered(c.listApps())
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (!usesCopy(e)) return ServerAppsResult.Failed(e)
            return readCopy()?.let { ServerAppsResult.Loaded(it, fromCache = true) } ?: ServerAppsResult.Failed(e)
        }
        writeCopy(apps, epoch)
        return ServerAppsResult.Loaded(apps, fromCache = false)
    }

    private suspend fun readCopy(): List<AppSummary>? =
        cache.loadJson(CACHE_KEY)?.let { runCatching { json.decodeFromJsonElement(list, json.parseToJsonElement(it).jsonObject.getValue("items")) }.getOrNull() }

    /** DocumentCache は版の古い物で上書きしないので、前の写しの版 + 1 で置く(`{"version", "items"}`)。読む間にサインアウトで全部消したら書かない。 */
    private suspend fun writeCopy(apps: List<AppSummary>, epoch: Int) {
        val previous = cache.loadJson(CACHE_KEY)?.let { runCatching { json.parseToJsonElement(it).jsonObject["version"]?.jsonPrimitive?.intOrNull }.getOrNull() } ?: 0
        val copy = buildJsonObject {
            put("version", previous + 1)
            put("items", json.encodeToJsonElement(list, apps))
        }
        cache.store(CACHE_KEY, copy.toString(), since = epoch)
    }

    companion object {
        /** DocumentCache の中の写しの名前(アプリの id と重ならない)。 */
        const val CACHE_KEY = "_server_apps"

        private val json = Json { ignoreUnknownKeys = true; encodeDefaults = true }
        private val list = ListSerializer(AppSummary.serializer())

        /** 一覧に出す並び(archived と未生成を除く・並べた順 → 更新の新しい順。iOS の `AppSummary.visible` と同じ)。 */
        fun ordered(apps: List<AppSummary>): List<AppSummary> = AppSummary.visible(apps, current = null)

        /** 写しで代える失敗: 圏外・時間切れ(IOException)と 5xx。 */
        fun usesCopy(e: Exception): Boolean = e is IOException || (e is ApiError && e.status >= 500)
    }
}
