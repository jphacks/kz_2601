package jp.kz2601.metamon.desktop.create

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.desktop.chat.ChatBuild
import jp.kz2601.metamon.desktop.chat.ChatEntry
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.MorphResult
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/** この起動で作ったアプリ(パレットのアプリの一覧に足す・AppHub はここから DSL を引く)。 */
class CreatedApps {
    private val dsls = LinkedHashMap<String, AppDSL>()

    /** (id, アプリ名)。作った順 */
    val all = mutableStateListOf<Pair<String, String>>()

    @Synchronized
    fun dsl(id: String): AppDSL? = dsls[id]

    @Synchronized
    fun add(id: String, dsl: AppDSL) {
        dsls[id] = dsl
        all.removeAll { it.first == id }
        all += id to dsl.app.name
    }
}

/**
 * 作る流れ(#2163)。会話の「これでいく」から 1 つずつ作る(作っている間の次の依頼は受けない)。
 * 完成したら [created] に足し、[cache] に封筒を残し、[onOpen] でそのアプリのウィンドウを開いて、作っている画面を閉じる。
 * 作っている間の「閉じる」は確認([confirmingClose])を出す。
 */
class AppCreator(
    private val source: GenerateSource,
    private val scope: CoroutineScope,
    val created: CreatedApps = CreatedApps(),
    private val locale: () -> String? = { null },
    private val cache: (suspend (MorphResult) -> Unit)? = null,
    private val onOpen: (String) -> Unit = {},
) {
    /** 作っている画面に出している物(無ければ null)。 */
    var current: AppCreation? by mutableStateOf(null)
        private set

    /** 作っている間に閉じようとした: 確認を出している。 */
    var confirmingClose: Boolean by mutableStateOf(false)
        private set

    internal var job: Job? = null
        private set

    /** 会話の ready・案から作り始める。作っている最中なら false(重ねない)。 */
    fun start(build: ChatBuild): Boolean {
        if (current?.state?.value?.isBuilding == true) return false
        val creation = AppCreation(source, build, build.appId.ifEmpty { ChatEntry.newAppId() }, locale)
        current = creation
        launch(creation)
        return true
    }

    /** 切れた・やり直せる失敗の後の「もう一度」。 */
    fun retry(): Boolean {
        val c = current ?: return false
        if (!c.state.value.canRetry) return false
        launch(c)
        return true
    }

    /** 閉じる操作(ウィンドウの ×・「閉じる」)。作っている間は確認を出す。 */
    fun requestClose() {
        if (current?.state?.value?.isBuilding == true) confirmingClose = true else dismiss()
    }

    /** 確認で「中止」: 作るのを止めて閉じる。 */
    fun stop() {
        confirmingClose = false
        job?.cancel()
        dismiss()
    }

    /** 確認で「続ける」。 */
    fun keepBuilding() {
        confirmingClose = false
    }

    private fun dismiss() {
        confirmingClose = false
        current = null
    }

    private fun launch(c: AppCreation) {
        job = scope.launch {
            val end = c.run()
            val result = end.result
            if (end.phase != CreatePhase.DONE || result == null || current !== c) return@launch
            val id = result.document.appId
            created.add(id, result.document.dsl)
            runCatching { cache?.invoke(result) }
            dismiss()
            onOpen(id)
        }
    }
}
