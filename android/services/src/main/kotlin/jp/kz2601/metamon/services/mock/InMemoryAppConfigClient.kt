package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.AppWithDsl
import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.MarkState
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.RecordPage
import jp.kz2601.metamon.services.RecordQuery
import jp.kz2601.metamon.services.Share
import jp.kz2601.metamon.services.SharePreview
import jp.kz2601.metamon.services.VersionSummary
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.JsonObject
import java.time.Instant
import java.util.UUID

/**
 * インメモリの appconfig(iOS の `InMemoryAppConfigClient`)。サーバー無しで全画面を動かすためのもの。
 * - DSL: アプリごとに最後に置いた本文を持つ。無ければ [initialDocument](既定はサンプルの initial)
 * - 記録: entity id → 記録。[seed] で初期値
 * - 版の履歴は持たない(取り消しは呼び出し側のローカル履歴に任せる・iOS と同じ)
 * - 共有: [sharing] が true の時だけ端末内で完結する固定トークンを返す(iOS `METAMON_MOCK_SHARING`)
 */
class InMemoryAppConfigClient(
    seed: Map<String, List<Record>> = emptyMap(),
    apps: List<AppSummary> = emptyList(),
    private val sharing: Boolean = false,
    private val initialDocument: (appId: String) -> FetchedDocument,
    private val now: () -> Instant = Instant::now,
) : AppConfigClient {
    private val mutex = Mutex()
    private val tables: MutableMap<String, MutableList<Record>> = seed.mapValues { it.value.toMutableList() }.toMutableMap()
    private val apps: MutableList<AppSummary> = apps.toMutableList()
    private val documents = mutableMapOf<String, FetchedDocument>()

    private fun stamp(): String = now().toString()

    private fun notFound(what: String): Nothing = throw ApiError.Server(404, "not_found", "$what not found (in-memory)")

    // アプリ

    override suspend fun listApps(): List<AppSummary> = mutex.withLock { apps.toList() }

    override suspend fun createApp(name: String?, dsl: JsonObject?): AppWithDsl = mutex.withLock {
        val id = UUID.randomUUID().toString().lowercase()
        val doc = dsl?.let { DslSamples.envelope(id, 1, it) } ?: initialDocument(id)
        documents[id] = doc
        val ts = stamp()
        val summary = AppSummary(id, name ?: doc.document.dsl.app.name, doc.version, createdAt = ts, updatedAt = ts)
        apps += summary
        withDsl(summary, doc)
    }

    override suspend fun renameApp(appId: String, name: String): AppSummary = mutex.withLock {
        val i = apps.indexOfFirst { it.id == appId }.takeIf { it >= 0 } ?: notFound("app")
        apps[i] = apps[i].copy(name = name, currentVersion = apps[i].currentVersion + 1, updatedAt = stamp())
        apps[i]
    }

    override suspend fun duplicateApp(appId: String, name: String?): AppWithDsl = mutex.withLock {
        val src = apps.firstOrNull { it.id == appId } ?: notFound("app")
        val id = UUID.randomUUID().toString().lowercase()
        val doc = documentFor(appId)
        val copy = DslSamples.envelope(id, 2, dslJson(doc))
        documents[id] = copy
        val ts = stamp()
        val summary = AppSummary(id, name ?: src.name, 2, createdAt = ts, updatedAt = ts)
        apps += summary
        withDsl(summary, copy)
    }

    override suspend fun deleteApp(appId: String) {
        mutex.withLock {
            apps.removeAll { it.id == appId }
            documents.remove(appId)
        }
    }

    override suspend fun reorderApps(ids: List<String>) {
        mutex.withLock {
            val byId = apps.associateBy { it.id }
            val ordered = ids.mapNotNull(byId::get).mapIndexed { i, a -> a.copy(sortIndex = i + 1) }
            val rest = apps.filter { it.id !in ids }.map { it.copy(sortIndex = null) }
            apps.clear()
            apps += ordered + rest
        }
    }

    // DSL

    override suspend fun fetchDocument(appId: String): FetchedDocument = mutex.withLock { documentFor(appId) }

    override suspend fun fetchDocumentIfChanged(appId: String, currentVersion: Int): FetchedDocument? =
        fetchDocument(appId).takeIf { it.version != currentVersion }

    override suspend fun putDocument(appId: String, dsl: JsonObject, baseVersion: Int, source: JsonObject?): VersionSummary =
        mutex.withLock {
            val current = documentFor(appId).version
            if (baseVersion != current) {
                throw ApiError.Server(409, "version_conflict", "version conflict (in-memory)", currentVersionDetails(current))
            }
            documents[appId] = DslSamples.envelope(appId, current + 1, dsl)
            VersionSummary(appId, current + 1, parentVersion = current, source = source, createdAt = stamp())
        }

    override suspend fun listVersions(appId: String): List<VersionSummary> = emptyList()

    override suspend fun restore(appId: String, version: Int): FetchedDocument = notFound("version")

    // 記録

    override suspend fun listRecords(appId: String, entity: String): List<Record> = mutex.withLock { tables[entity].orEmpty().toList() }

    /** filter・並びは解釈しない(全件・1 ページ)。 */
    override suspend fun queryRecords(appId: String, entity: String, query: RecordQuery): RecordPage =
        RecordPage(listRecords(appId, entity))

    override suspend fun queryAllRecords(appId: String, entity: String, query: RecordQuery): List<Record> =
        listRecords(appId, entity)

    override suspend fun createRecord(appId: String, entity: String, data: JsonObject, clientId: String, authorName: String?): Record =
        mutex.withLock {
            val rows = tables.getOrPut(entity) { mutableListOf() }
            // clientId で冪等(サーバーと同じ: 同じ clientId の再送は前の記録を返す)
            rows.firstOrNull { it.id == clientId }?.let { return@withLock it }
            val ts = stamp()
            val record = Record(
                id = clientId, entity = entity, data = data,
                author = authorName?.let { MockAuthService.FIXED_UID }, authorName = authorName,
                createdAt = ts, updatedAt = ts,
            )
            rows += record
            record
        }

    override suspend fun updateRecord(appId: String, entity: String, id: String, data: JsonObject): Record = mutex.withLock {
        val rows = tables[entity] ?: notFound("$entity/$id")
        val i = rows.indexOfFirst { it.id == id }.takeIf { it >= 0 } ?: notFound("$entity/$id")
        rows[i] = rows[i].copy(data = JsonObject(rows[i].data + data), updatedAt = stamp())
        rows[i]
    }

    override suspend fun deleteRecord(appId: String, entity: String, id: String) {
        mutex.withLock { tables[entity]?.removeAll { it.id == id } }
    }

    /** 印(#255)。モックは利用者が 1 人なので、その 1 人分だけ増減する。 */
    override suspend fun setMark(appId: String, entity: String, id: String, mark: String, on: Boolean): MarkState = mutex.withLock {
        val rows = tables[entity] ?: notFound("record")
        val i = rows.indexOfFirst { it.id == id }.takeIf { it >= 0 } ?: notFound("record")
        val st = rows[i].marks?.get(mark) ?: MarkState()
        val next = if (st.mine == on) st else MarkState(count = (st.count + if (on) 1 else -1).coerceAtLeast(0), mine = on)
        rows[i] = rows[i].copy(marks = rows[i].marks.orEmpty() + (mark to next))
        next
    }

    // 共有

    override suspend fun share(appId: String): Share {
        requireSharing()
        val name = mutex.withLock { apps.firstOrNull { it.id == appId }?.name } ?: appId
        return Share(
            token = SHARE_TOKEN, appId = appId, name = name, dslVersion = 1,
            url = "tailorit://import/$SHARE_TOKEN", webUrl = "https://example.com/s/$SHARE_TOKEN",
        )
    }

    override suspend fun previewShare(token: String): SharePreview {
        requireSharing()
        return SharePreview(token = token, name = "インポート", components = listOf("list", "form"))
    }

    override suspend fun importShare(token: String, displayName: String?): InstallResult {
        requireSharing()
        val created = createApp(name = "インポート")
        return InstallResult(id = created.id, name = created.name, currentVersion = created.currentVersion, dsl = created.dsl)
    }

    override suspend fun revokeShare(token: String) = requireSharing()

    private fun requireSharing() {
        if (!sharing) notFound("share")
    }

    private fun documentFor(appId: String): FetchedDocument = documents.getOrPut(appId) { initialDocument(appId) }

    private fun dslJson(doc: FetchedDocument): JsonObject =
        (kotlinx.serialization.json.Json.parseToJsonElement(doc.envelopeJson) as JsonObject)["dsl"] as JsonObject

    private fun withDsl(s: AppSummary, doc: FetchedDocument) =
        AppWithDsl(s.id, s.name, s.currentVersion, s.status, s.createdAt, s.updatedAt, s.sortIndex, dslJson(doc))

    private fun currentVersionDetails(v: Int) =
        JsonObject(mapOf("currentVersion" to kotlinx.serialization.json.JsonPrimitive(v)))

    companion object {
        const val SHARE_TOKEN = "uitest-token"
    }
}
