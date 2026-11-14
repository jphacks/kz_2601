package jp.kz2601.metamon.voice

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.VersionSummary
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull

/** 生成したアプリの版(appconfig の `GET /versions`・`POST /versions/{n}/restore`)。テストで差し替える。 */
interface AppVersions {
    /** 版の履歴(新しい順)。サーバーが履歴を返せなければ空 */
    suspend fun versions(appId: String): List<VersionInfo>

    /** その版の内容で新しい版を作り、復元後の DSL を返す */
    suspend fun restore(appId: String, version: Int): AppDocument
}

class AppConfigVersions(private val client: AppConfigClient) : AppVersions {
    override suspend fun versions(appId: String): List<VersionInfo> = client.listVersions(appId).map { it.info }

    override suspend fun restore(appId: String, version: Int): AppDocument = client.restore(appId, version).document
}

/** `source` は素の JSON なので、「元に戻す」が見る `kind` / `restoredFrom` だけ取り出す。 */
private val VersionSummary.info: VersionInfo
    get() = VersionInfo(
        version = version,
        parentVersion = parentVersion,
        kind = (source?.get("kind") as? JsonPrimitive)?.contentOrNull,
        restoredFrom = (source?.get("restoredFrom") as? JsonPrimitive)?.intOrNull,
    )

/** 要約のバナーに添えるボタン(iOS の `SummaryBanner.Action`)。修正の直後は UNDO、戻した直後は REDO。 */
enum class UndoAction { UNDO, REDO }

/** undo / redo の結果をトーストで伝える(iOS の `toast = L10n.tr(...)`)。 */
enum class UndoNotice { NO_EARLIER, REDO_FAILED }

/** 「やり直す」の戻り先。戻す直前の中身の版。サーバー経由なら [version] を restore、端末の履歴なら [document] を当て直す。 */
internal data class RedoPoint(val version: Int, val document: AppDocument, val viaServer: Boolean)
