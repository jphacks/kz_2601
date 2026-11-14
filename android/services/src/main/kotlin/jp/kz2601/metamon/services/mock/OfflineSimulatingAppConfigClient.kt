package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.MarkState
import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.RecordPage
import jp.kz2601.metamon.services.RecordQuery
import kotlinx.serialization.json.JsonObject
import java.time.Instant
import java.util.Collections

/**
 * 機内モードの擬似(iOS の `OfflineSimulatingAppConfigClient`)。[offlineUntil] まで記録の API を圏外の例外で落とす(DSL の取得は通す)。
 * `METAMON_E2E_OFFLINE_SECONDS` とテストで、圏外で保存 → 復帰で送る経路を通すために使う。
 * 落とす例外は [java.net.UnknownHostException](`FailureKind.classify` で Offline になる)。
 */
class OfflineSimulatingAppConfigClient(
    private val inner: AppConfigClient,
    @Volatile private var offlineUntil: Instant? = null,
    private val now: () -> Instant = Instant::now,
) : AppConfigClient by inner {
    /** 通した / 落とした呼び出しの記録(テスト用) */
    val log: MutableList<String> = Collections.synchronizedList(mutableListOf())

    val isOffline: Boolean get() = offlineUntil?.isAfter(now()) ?: false

    fun setOffline(offline: Boolean) {
        offlineUntil = if (offline) Instant.MAX else null
    }

    private fun gate(name: String) {
        val offline = isOffline
        log += (if (offline) "drop " else "pass ") + name
        if (offline) throw java.net.UnknownHostException("offline (simulated)")
    }

    override suspend fun listRecords(appId: String, entity: String): List<Record> {
        gate("list $entity")
        return inner.listRecords(appId, entity)
    }

    override suspend fun queryRecords(appId: String, entity: String, query: RecordQuery): RecordPage {
        gate("query $entity")
        return inner.queryRecords(appId, entity, query)
    }

    override suspend fun queryAllRecords(appId: String, entity: String, query: RecordQuery): List<Record> {
        gate("query $entity")
        return inner.queryAllRecords(appId, entity, query)
    }

    override suspend fun createRecord(appId: String, entity: String, data: JsonObject, clientId: String, authorName: String?): Record {
        gate("create $entity $clientId")
        return inner.createRecord(appId, entity, data, clientId, authorName)
    }

    override suspend fun updateRecord(appId: String, entity: String, id: String, data: JsonObject): Record {
        gate("update $entity/$id")
        return inner.updateRecord(appId, entity, id, data)
    }

    override suspend fun deleteRecord(appId: String, entity: String, id: String) {
        gate("delete $entity/$id")
        inner.deleteRecord(appId, entity, id)
    }

    override suspend fun deleteAllRecords(appId: String, entity: String) {
        gate("delete-all $entity")
        inner.deleteAllRecords(appId, entity)
    }

    override suspend fun setMark(appId: String, entity: String, id: String, mark: String, on: Boolean): MarkState {
        gate("mark $entity/$id $mark $on")
        return inner.setMark(appId, entity, id, mark, on)
    }
}
