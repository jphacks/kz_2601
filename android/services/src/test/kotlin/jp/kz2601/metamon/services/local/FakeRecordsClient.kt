package jp.kz2601.metamon.services.local

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
import kotlinx.serialization.json.JsonObject
import java.io.IOException

/** 記録の API だけを持つ偽物。`offline` の間は通信断、`failures` に積んだ例外を順に投げる。 */
class FakeRecordsClient : AppConfigClient {
    data class Call(val kind: String, val entity: String, val id: String?, val data: JsonObject?, val clientId: String? = null)

    val calls = mutableListOf<Call>()
    val failures = ArrayDeque<Throwable>()
    var offline = false
    var server = mutableListOf<Record>()
    private var next = 1

    private fun gate() {
        if (offline) throw IOException("offline")
        failures.removeFirstOrNull()?.let { throw it }
    }

    override suspend fun listRecords(appId: String, entity: String): List<Record> {
        calls += Call("list", entity, null, null)
        gate()
        return server.toList()
    }

    override suspend fun createRecord(appId: String, entity: String, data: JsonObject, clientId: String, authorName: String?): Record {
        calls += Call("create", entity, null, data, clientId)
        gate()
        return Record(id = "srv-${next++}", entity = entity, data = data, createdAt = "2026-11-11T00:00:00Z").also { server += it }
    }

    override suspend fun updateRecord(appId: String, entity: String, id: String, data: JsonObject): Record {
        calls += Call("update", entity, id, data)
        gate()
        return Record(id = id, entity = entity, data = data, createdAt = "2026-11-11T00:00:00Z")
    }

    override suspend fun deleteRecord(appId: String, entity: String, id: String) {
        calls += Call("delete", entity, id, null)
        gate()
        server.removeAll { it.id == id }
    }

    override suspend fun setMark(appId: String, entity: String, id: String, mark: String, on: Boolean): MarkState {
        gate()
        return MarkState(count = if (on) 1 else 0, mine = on)
    }

    override suspend fun listApps(): List<AppSummary> = TODO()
    override suspend fun createApp(name: String?, dsl: JsonObject?): AppWithDsl = TODO()
    override suspend fun renameApp(appId: String, name: String): AppSummary = TODO()
    override suspend fun duplicateApp(appId: String, name: String?): AppWithDsl = TODO()
    override suspend fun deleteApp(appId: String) = TODO()
    override suspend fun reorderApps(ids: List<String>) = TODO()
    override suspend fun fetchDocument(appId: String): FetchedDocument = TODO()
    override suspend fun fetchDocumentIfChanged(appId: String, currentVersion: Int): FetchedDocument? = TODO()
    override suspend fun putDocument(appId: String, dsl: JsonObject, baseVersion: Int, source: JsonObject?): VersionSummary = TODO()
    override suspend fun listVersions(appId: String): List<VersionSummary> = TODO()
    override suspend fun restore(appId: String, version: Int): FetchedDocument = TODO()
    override suspend fun queryRecords(appId: String, entity: String, query: RecordQuery): RecordPage = TODO()
    override suspend fun queryAllRecords(appId: String, entity: String, query: RecordQuery): List<Record> = TODO()
    override suspend fun share(appId: String): Share = TODO()
    override suspend fun previewShare(token: String): SharePreview = TODO()
    override suspend fun importShare(token: String, displayName: String?): InstallResult = TODO()
    override suspend fun revokeShare(token: String) = TODO()
}
