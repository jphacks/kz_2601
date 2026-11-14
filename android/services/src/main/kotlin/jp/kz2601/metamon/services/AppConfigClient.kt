package jp.kz2601.metamon.services

import jp.kz2601.metamon.dsl.AppDocument
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

/**
 * appconfig サービス(iOS の `AppConfigClient`)。契約: docs/backend/openapi-appconfig.yaml。
 * DSL の本文は書き込みでは [JsonObject] のまま扱う(:dsl の型は知らないキーを落とすので、往復で消さないため)。
 */
interface AppConfigClient {
    // アプリ

    /** `GET /v1/apps`(最大 5 ページ)。 */
    suspend fun listApps(): List<AppSummary>

    /** `POST /v1/apps`。`dsl` を省略するとサーバーの初期 DSL。 */
    suspend fun createApp(name: String? = null, dsl: JsonObject? = null): AppWithDsl

    /** `PATCH /v1/apps/{appId}`(名前・#277)。 */
    suspend fun renameApp(appId: String, name: String): AppSummary

    /** `POST /v1/apps/{appId}/duplicate`(#277)。記録は複製しない。 */
    suspend fun duplicateApp(appId: String, name: String? = null): AppWithDsl

    /** `DELETE /v1/apps/{appId}`(物理削除・#180)。 */
    suspend fun deleteApp(appId: String)

    /** `PUT /v1/apps/order`(#277)。 */
    suspend fun reorderApps(ids: List<String>)

    // DSL

    /** `GET /v1/apps/{appId}/dsl`。未知の appId はサーバーが version 1 で作る。 */
    suspend fun fetchDocument(appId: String): FetchedDocument

    /** `If-None-Match: "<currentVersion>"` 付きの取得。304 か同じ version なら null。 */
    suspend fun fetchDocumentIfChanged(appId: String, currentVersion: Int): FetchedDocument?

    /**
     * `PUT /v1/apps/{appId}/dsl`(`If-Match: "<baseVersion>"`)。
     * 版がずれていれば 409 `version_conflict`(`ApiError.currentVersion`)、If-Match が無ければ 412。
     */
    suspend fun putDocument(appId: String, dsl: JsonObject, baseVersion: Int, source: JsonObject? = null): VersionSummary

    /** `GET /v1/apps/{appId}/versions`(新しい順・最大 50)。 */
    suspend fun listVersions(appId: String): List<VersionSummary>

    /** `POST /v1/apps/{appId}/versions/{version}/restore`。復元後の DSL を返す。 */
    suspend fun restore(appId: String, version: Int): FetchedDocument

    // 記録

    /** `GET .../records`(filter なし・最大 5 ページ)。 */
    suspend fun listRecords(appId: String, entity: String): List<Record>

    /** `POST .../records/query`(1 ページ)。filter の値を URL に載せない(#818)。 */
    suspend fun queryRecords(appId: String, entity: String, query: RecordQuery = RecordQuery()): RecordPage

    /** `POST .../records/query` を最大 5 ページまで続けて読む。 */
    suspend fun queryAllRecords(appId: String, entity: String, query: RecordQuery = RecordQuery()): List<Record>

    /** `POST .../records`。`clientId` で冪等(通信断の再送で二重登録しない)。 */
    suspend fun createRecord(
        appId: String,
        entity: String,
        data: JsonObject,
        clientId: String = java.util.UUID.randomUUID().toString(),
        authorName: String? = null,
    ): Record

    suspend fun updateRecord(appId: String, entity: String, id: String, data: JsonObject): Record

    suspend fun deleteRecord(appId: String, entity: String, id: String)

    /**
     * `DELETE .../records?confirm=true`(エンティティの全記録の削除・デモのやり直し用)。
     * public/shared は自分の投稿だけ消える。`deleteBy: admins` で管理者でなければ 403 `admins_only`。
     */
    suspend fun deleteAllRecords(appId: String, entity: String)

    /** `PUT` / `DELETE .../records/{id}/marks/{mark}`(#255)。 */
    suspend fun setMark(appId: String, entity: String, id: String, mark: String, on: Boolean): MarkState

    // 共有

    /** `POST /v1/apps/{appId}/share`。 */
    suspend fun share(appId: String): Share

    /** `GET /v1/shares/{token}`(取り込む前の確認)。 */
    suspend fun previewShare(token: String): SharePreview

    /** `POST /v1/shares/{token}/import`。 */
    suspend fun importShare(token: String, displayName: String? = null): InstallResult

    /** `DELETE /v1/shares/{token}`(発行者のみ・冪等)。 */
    suspend fun revokeShare(token: String)
}

/** DSL の封筒。[envelopeJson] はキャッシュ用にそのまま持つ(知らないキーも残る)。 */
data class FetchedDocument(val document: AppDocument, val envelopeJson: String) {
    val version: Int get() = document.version
}

@Serializable
data class AppSummary(
    val id: String,
    val name: String = "",
    val currentVersion: Int = 0,
    /** active | archived */
    val status: String = "active",
    val createdAt: String? = null,
    val updatedAt: String? = null,
    /** 変身一覧で自分が並べた位置(1 が先頭)。並べ替えていないアプリは null */
    val sortIndex: Int? = null,
) {
    val isArchived: Boolean get() = status == "archived"

    /** `GET /dsl` の自動作成(version 1 = 初期状態)のまま一度も生成されていない。一覧には出さない。 */
    val isBlank: Boolean get() = currentVersion <= 1

    /** [updatedAt] を時刻にしたもの(読めなければ null)。 */
    val updatedInstant: java.time.Instant? get() = updatedAt?.let { runCatching { java.time.OffsetDateTime.parse(it).toInstant() }.getOrNull() }

    companion object {
        /**
         * 一覧に出す並び(iOS の `AppSummary.visible`): archived と未生成(blank)を除く(今のアプリは常に含める)。
         * - 自分で並べたことがある(どれかに sortIndex がある・#277): sortIndex なしを新しい順で先頭、続けて並べた順
         * - 無い: 今のアプリを先頭、残りは更新の新しい順
         */
        fun visible(apps: List<AppSummary>, current: String?): List<AppSummary> {
            val kept = apps.distinctBy { it.id }.filter { !it.isArchived && (it.id == current || !it.isBlank) }
            val newer = compareByDescending<AppSummary> { it.updatedInstant ?: java.time.Instant.MIN }
            if (kept.any { it.sortIndex != null }) {
                return kept.sortedWith(compareBy<AppSummary> { it.sortIndex != null }.thenBy { it.sortIndex ?: 0 }.then(newer))
            }
            return kept.sortedWith(compareBy<AppSummary> { it.id != current }.then(newer))
        }

        /** 一覧の先頭のアプリ(今のアプリを除く・iOS の `latestApp`・#276 の開き直し)。 */
        fun latest(apps: List<AppSummary>, excluding: String): AppSummary? =
            visible(apps, current = null).firstOrNull { it.id != excluding }
    }
}

/** `AppWithDSL`(作成・複製の応答)。 */
@Serializable
data class AppWithDsl(
    val id: String,
    val name: String = "",
    val currentVersion: Int = 0,
    val status: String = "active",
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val sortIndex: Int? = null,
    val dsl: JsonObject,
) {
    val summary: AppSummary get() = AppSummary(id, name, currentVersion, status, createdAt, updatedAt, sortIndex)
}

@Serializable
data class VersionSummary(
    val appId: String,
    val version: Int,
    val parentVersion: Int? = null,
    val source: JsonObject? = null,
    val createdAt: String? = null,
)

@Serializable
data class MarkState(val count: Int = 0, val mine: Boolean = false)

@Serializable
data class Record(
    val id: String,
    val entity: String = "",
    val data: JsonObject = JsonObject(emptyMap()),
    /** 投稿者 uid(public / shared のみ) */
    val author: String? = null,
    val authorName: String? = null,
    val marks: Map<String, MarkState>? = null,
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

/** `POST .../records/query` の本文。null のキーは送らない(`{}` = filter なし・既定の並び)。 */
@Serializable
data class RecordQuery(
    val filterField: String? = null,
    /** eq | neq | gt | gte | lt | lte | thisMonth | today | in | nin */
    val filterOp: String? = null,
    val filterValue: String? = null,
    val sortField: String? = null,
    /** asc | desc */
    val sortDir: String? = null,
    val pageSize: Int? = null,
    val pageToken: String? = null,
)

@Serializable
data class RecordPage(val items: List<Record> = emptyList(), val nextPageToken: String? = null)

@Serializable
data class Share(
    val token: String,
    val appId: String,
    val name: String = "",
    val dslVersion: Int = 0,
    /** アプリで開く URL(QR もこれ) */
    val url: String = "",
    /** ブラウザ用の共有ページ */
    val webUrl: String = "",
    val revoked: Boolean? = null,
    val createdAt: String? = null,
)

@Serializable
data class SharePreview(
    val token: String = "",
    val name: String = "",
    val description: String? = null,
    val dslVersion: Int? = null,
    val components: List<String> = emptyList(),
    /** 取り込むと元のアプリの共有データに参加する */
    val sharedData: Boolean = false,
    val mine: Boolean = false,
)

/** `InstallResult`(取り込みの応答)= AppWithDSL + 複製の結果。 */
@Serializable
data class InstallResult(
    val id: String,
    val name: String = "",
    val currentVersion: Int = 0,
    val status: String = "active",
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val dsl: JsonObject,
    /** false = 既に持っていた(参加済み) */
    val created: Boolean = true,
    /** true = 共有データ付きなので元の appId に参加した */
    val joined: Boolean = false,
    /** true = scope:shared の参加者になった */
    val member: Boolean = false,
    /** true = 呼び出した本人が元の作成者か発行者(課金画面を出さない) */
    val mine: Boolean = false,
) {
    val summary: AppSummary get() = AppSummary(id, name, currentVersion, status, createdAt, updatedAt)
}
