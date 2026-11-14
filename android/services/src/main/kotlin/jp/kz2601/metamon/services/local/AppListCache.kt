package jp.kz2601.metamon.services.local

import jp.kz2601.metamon.services.AppSummary
import kotlinx.serialization.builtins.ListSerializer
import java.io.File

/**
 * 最後に取得したアプリの一覧(iOS の `AppListCache`)。オフラインで一覧を開いた時だけ使う(正はサーバーの `GET /v1/apps`)。
 * 置き場は `filesDir/metamon-app/apps.json`(DSL の置き場と分ける: appId が "apps" のアプリとぶつけない)。
 *
 * @param directory 置き場。null ならメモリだけ(テスト用)
 */
class AppListCache(private val directory: File?) {
    private val lock = Any()
    private var memory: List<AppSummary>? = null
    private val serializer = ListSerializer(AppSummary.serializer())

    fun store(apps: List<AppSummary>) {
        synchronized(lock) {
            memory = apps
            val file = file() ?: return
            runCatching { AtomicFile.write(file, RecordStore.json.encodeToString(serializer, apps)) }
                .onFailure { println("[Metamon] AppListCache store failed: $it") }
        }
    }

    /** 保存した一覧。無いか壊れていれば空。 */
    fun load(): List<AppSummary> = synchronized(lock) {
        memory ?: run {
            val file = file()
            val apps = runCatching {
                if (file != null && file.exists()) RecordStore.json.decodeFromString(serializer, file.readText()) else null
            }.getOrNull().orEmpty()
            memory = apps
            apps
        }
    }

    /** アカウント削除(#119)。 */
    fun clear() {
        synchronized(lock) {
            memory = null
            file()?.delete()
        }
    }

    private fun file(): File? = directory?.resolve("apps.json")
}
