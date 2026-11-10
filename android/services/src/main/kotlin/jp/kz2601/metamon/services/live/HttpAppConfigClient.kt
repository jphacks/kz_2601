package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.AppWithDsl
import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.MarkState
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.RecordPage
import jp.kz2601.metamon.services.RecordQuery
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.SharePreview
import jp.kz2601.metamon.services.VersionSummary
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.serialization.Serializable
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.io.IOException

/**
 * appconfig の REST 実装(iOS の `HTTPAppConfigClient`)。
 * - `GET /dsl` は `ETag: "<version>"`。`If-None-Match` で 304 なら変化なし
 * - DSL の書き込みは `If-Match: "<baseVersion>"` 必須(無いと 412・ずれると 409)
 * - 一覧は `pageSize` 200 × 最大 5 ページ
 * - 取得した DSL の封筒は [DocumentCache] に入れる(#883)
 */
class HttpAppConfigClient(
    private val http: MetamonHttp,
    /** 取得・復元した DSL の封筒を入れる(オフライン起動の復元用・iOS と同じ)。null なら入れない */
    private val cache: DocumentCache? = null,
) : AppConfigClient, PlanUsageSource {

    @Serializable
    private data class AppsPage(val items: List<AppSummary> = emptyList(), val nextPageToken: String? = null)

    @Serializable
    private data class VersionsPage(val items: List<VersionSummary> = emptyList())

    // MARK: アプリ

    override suspend fun listApps(): List<AppSummary> = paged { token ->
        val page = decode<AppsPage>(get(apps(), pageQuery(token)))
        page.items to page.nextPageToken
    }

    override suspend fun createApp(name: String?, dsl: JsonObject?): AppWithDsl {
        val body = buildJsonObject {
            name?.let { put("name", it) }
            dsl?.let { put("dsl", it) }
        }
        return decode(send("POST", apps(), body.toString()))
    }

    override suspend fun renameApp(appId: String, name: String): AppSummary =
        decode(send("PATCH", apps(appId), nameBody(name)))

    override suspend fun duplicateApp(appId: String, name: String?): AppWithDsl =
        decode(send("POST", apps(appId, "duplicate"), name?.let(::nameBody) ?: "{}"))

    override suspend fun deleteApp(appId: String) {
        send("DELETE", apps(appId))
    }

    override suspend fun reorderApps(ids: List<String>) {
        send("PUT", apps("order"), buildJsonObject { put("ids", JsonArray(ids.map { JsonPrimitive(it) })) }.toString())
    }

    // MARK: DSL

    override suspend fun fetchDocument(appId: String): FetchedDocument {
        val epoch = cache?.epoch
        return envelope(get(apps(appId, "dsl")), epoch)
    }

    override suspend fun fetchDocumentIfChanged(appId: String, currentVersion: Int): FetchedDocument? {
        val epoch = cache?.epoch
        val r = http.exchange(HttpRequest("GET", apps(appId, "dsl"), headers = mapOf("If-None-Match" to etag(currentVersion))))
        if (r.status == 304) return null
        val doc = envelope(http.check(r), epoch)
        return doc.takeIf { it.version != currentVersion }
    }

    override suspend fun putDocument(appId: String, dsl: JsonObject, baseVersion: Int, source: JsonObject?): VersionSummary {
        val body = buildJsonObject {
            put("dsl", dsl)
            source?.let { put("source", it) }
        }
        val req = HttpRequest("PUT", apps(appId, "dsl"), body = body.toString(), headers = mapOf("If-Match" to etag(baseVersion)))
        return decode(http.send(req))
    }

    override suspend fun listVersions(appId: String): List<VersionSummary> =
        decode<VersionsPage>(get(apps(appId, "versions"), listOf("pageSize" to "50"))).items

    override suspend fun restore(appId: String, version: Int): FetchedDocument {
        val epoch = cache?.epoch
        // 201 の本文は VersionSummary + 復元後の dsl(DSLEnvelope 互換)
        return envelope(send("POST", apps(appId, "versions", version.toString(), "restore")), epoch)
    }

    // MARK: 記録

    override suspend fun listRecords(appId: String, entity: String): List<Record> = paged { token ->
        val page = decode<RecordPage>(get(records(appId, entity), pageQuery(token)))
        page.items to page.nextPageToken
    }

    override suspend fun queryRecords(appId: String, entity: String, query: RecordQuery): RecordPage =
        decode(send("POST", records(appId, entity, "query"), ApiJson.encodeToString(RecordQuery.serializer(), query)))

    override suspend fun queryAllRecords(appId: String, entity: String, query: RecordQuery): List<Record> = paged { token ->
        val page = queryRecords(appId, entity, query.copy(pageSize = query.pageSize ?: PAGE_SIZE, pageToken = token))
        page.items to page.nextPageToken
    }

    override suspend fun createRecord(appId: String, entity: String, data: JsonObject, clientId: String, authorName: String?): Record {
        val body = buildJsonObject {
            put("data", data)
            put("clientId", clientId)
            authorName?.let { put("authorName", it) }
        }.toString()
        return try {
            decode(send("POST", records(appId, entity), body))
        } catch (e: IOException) {
            // 送ったか分からない通信断。同じ clientId で再送すればサーバーが同じ記録に畳む
            decode(send("POST", records(appId, entity), body))
        }
    }

    override suspend fun updateRecord(appId: String, entity: String, id: String, data: JsonObject): Record =
        decode(send("PATCH", records(appId, entity, id), buildJsonObject { put("data", data) }.toString()))

    override suspend fun deleteRecord(appId: String, entity: String, id: String) {
        send("DELETE", records(appId, entity, id))
    }

    override suspend fun setMark(appId: String, entity: String, id: String, mark: String, on: Boolean): MarkState =
        decode(send(if (on) "PUT" else "DELETE", records(appId, entity, id, "marks", mark)))

    // MARK: 共有

    override suspend fun share(appId: String): Share = decode(send("POST", apps(appId, "share")))

    override suspend fun previewShare(token: String): SharePreview = decode(get(listOf("v1", "shares", token)))

    override suspend fun importShare(token: String, displayName: String?): InstallResult {
        val body = buildJsonObject { displayName?.let { put("displayName", it) } }.toString()
        return decode(send("POST", listOf("v1", "shares", token, "import"), body))
    }

    override suspend fun revokeShare(token: String) {
        send("DELETE", listOf("v1", "shares", token))
    }

    // MARK: 使用量(#630・iOS の `HTTPAppConfigClient+Usage`)

    /** `GET /v1/me/usage`。古いサーバー(まだ無い)は 404 → null */
    override suspend fun myUsage(): MyUsage? = try {
        MyUsage.decode(get(listOf("v1", "me", "usage")))
    } catch (e: ApiError) {
        if (e.status == 404) null else throw e
    }

    /** `GET /v1/apps/{appId}/usage`。古いサーバー・このアプリの写しが無い時は 404 → null */
    override suspend fun appUsage(appId: String): AppUsage? = try {
        AppUsage.decode(get(listOf("v1", "apps", appId, "usage")))
    } catch (e: ApiError) {
        if (e.status == 404) null else throw e
    }

    // MARK: 下回り

    private suspend fun get(path: List<String>, query: List<Pair<String, String>> = emptyList()): String =
        http.send(HttpRequest("GET", path, query))

    private suspend fun send(method: String, path: List<String>, body: String? = null): String =
        http.send(HttpRequest(method, path, body = body))

    /** [epoch] = 送る前の [DocumentCache.epoch]。その間にサインアウトの後片付けが走っていたら書き戻さない。 */
    private suspend fun envelope(json: String, epoch: Int?): FetchedDocument {
        val doc = FetchedDocument(AppDocument.decode(json), json)
        cache?.store(doc.document.appId, doc.envelopeJson, since = epoch)
        return doc
    }

    private inline fun <reified T> decode(json: String): T = ApiJson.decodeFromString(json)

    private fun nameBody(name: String) = buildJsonObject { put("name", name) }.toString()

    private suspend fun <T> paged(fetch: suspend (String?) -> Pair<List<T>, String?>): List<T> {
        val out = mutableListOf<T>()
        var token: String? = null
        repeat(MAX_PAGES) {
            val (items, next) = fetch(token)
            out += items
            if (next.isNullOrEmpty()) return out
            token = next
        }
        return out
    }

    companion object {
        const val PAGE_SIZE = 200

        /** 1 entity あたり最大 1000 件まで(それ以上は画面でも持て余す) */
        const val MAX_PAGES = 5

        /** backend の `etag(v)` と同じ `"7"` 形式 */
        fun etag(version: Int): String = "\"$version\""

        private fun apps(vararg rest: String) = listOf("v1", "apps") + rest
        private fun records(appId: String, entity: String, vararg rest: String) =
            apps(appId, "entities", entity, "records") + rest

        private fun pageQuery(token: String?) = buildList {
            add("pageSize" to PAGE_SIZE.toString())
            token?.let { add("pageToken" to it) }
        }
    }
}
