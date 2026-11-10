package jp.kz2601.metamon.services.sync

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.services.FetchedDocument
import jp.kz2601.metamon.services.http.MINI_DSL
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.test.TestCoroutineScheduler

fun envelope(version: Int, appId: String = "app1") = """{"appId":"$appId","version":$version,"dsl":$MINI_DSL}"""

fun fetched(version: Int, appId: String = "app1"): FetchedDocument {
    val json = envelope(version, appId)
    return FetchedDocument(AppDocument.decode(json), json)
}

/** サーバーの今の版を持ち、手元より新しければ返す。呼ばれた時刻を残す。 */
class FakeFetcher(private val scheduler: TestCoroutineScheduler, var serverVersion: Int = 1) : DocumentFetcher {
    val calls = mutableListOf<Long>()
    var failing = false

    override suspend fun fetchIfChanged(appId: String, currentVersion: Int): FetchedDocument? {
        calls += scheduler.currentTime
        if (failing) throw java.io.IOException("offline")
        return if (serverVersion > currentVersion) fetched(serverVersion, appId) else null
    }
}

/** 購読 1 本ごとの台本。尽きたら切れずにつながったまま(cancel まで待つ)。 */
class FakeEventSource(private val scheduler: TestCoroutineScheduler) : ChangeEventSource {
    val sessions = ArrayDeque<suspend (suspend (ChangeEvent) -> Unit) -> Boolean>()

    /** 張った時刻と forceRefresh */
    val connects = mutableListOf<Pair<Long, Boolean>>()

    fun then(script: suspend (suspend (ChangeEvent) -> Unit) -> Boolean) = apply { sessions.addLast(script) }

    override suspend fun stream(appId: String, forceRefresh: Boolean, onEvent: suspend (ChangeEvent) -> Unit): Boolean {
        connects += scheduler.currentTime to forceRefresh
        val script = sessions.removeFirstOrNull() ?: awaitCancellation()
        return script(onEvent)
    }
}
