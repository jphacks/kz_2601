package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyShare
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 近距離共有(#1382)とアプリのランタイムをつなぐ(iOS の `AppRuntime+Nearby` と `NearbyUI`)。
 * - 部品から: 送るデータを作って相手選びのシートを開く([share])
 * - 受信: 届いた申し出を今のアプリで判定し、承認された記録を足す([import])
 * - 名前: サインインの表示名 → 端末の名前。「名前を出さない」はすれ違い通信と同じ設定([NearbySettings])
 */
class NearbyShareController(
    private val runtime: AppRuntime,
    val service: NearbyService,
    private val settings: NearbySettings,
    /** 自分の名前(相手の一覧と「◯◯さんから」に出る)。投稿の名前 → 端末の名前 */
    private val displayName: () -> String,
) {
    data class SendRequest(val id: Long, val payload: NearbyShare.Payload)

    private val _request = MutableStateFlow<SendRequest?>(null)

    /** 送信シート(相手選び)。null でなければシートを出す。 */
    val request: StateFlow<SendRequest?> = _request.asStateFlow()
    private var seq = 0L

    private val _anonymous = MutableStateFlow(settings.anonymous)
    val anonymous: StateFlow<Boolean> = _anonymous.asStateFlow()

    /** この起動で近くのデバイスの権限を一度聞いたか(待ち受けのために自分から聞くのは 1 回だけ)。 */
    var askedForPermission = false

    init {
        service.rename(publicName)
        service.decide = { header -> decide(header) }
        service.onReceive = { payload, _ -> import(payload) }
        runtime.nearby = this
    }

    /** 相手に見える名前(端末の名前と Header の送り主)。 */
    val publicName: String get() = NearbyLimits.peerName(displayName(), _anonymous.value)

    fun share(entity: String, records: List<Map<String, JSONValue>>) {
        val e = runtime.entityModel(entity) ?: return
        val payload = NearbyShare.makePayload(service.myName.value, runtime.document.appId, runtime.dsl.app.name, e, records)
        if (payload.header.count == 0) {
            runtime.showToast(UiText.Res(jp.kz2601.metamon.renderer.R.string.nearby_nothing_to_send))
            return
        }
        _request.value = SendRequest(++seq, payload)
    }

    fun dismissRequest() {
        _request.value = null
    }

    /** 「名前を出さない」の切り替え(すれ違い通信と同じ設定)。名前を変えて待ち受け・探すのをやり直す。 */
    fun setAnonymous(on: Boolean) {
        if (on == _anonymous.value) return
        _anonymous.value = on
        settings.anonymous = on
        service.rename(publicName)
    }

    /** 前面にいて、今のアプリが送る `nearbyShare` を使い、権限がある間だけ待ち受ける。 */
    fun updateAdvertising(active: Boolean) {
        service.setAdvertising(active && runtime.dsl.usesNearbyShare)
    }

    internal fun decide(header: NearbyShare.Header): NearbyShare.Decision =
        // 曲のリクエスト(#412)は Android の musicPlayer が受けないので断る
        NearbyShare.decide(header, runtime.entityModels, usesMusicRequests = false, appId = runtime.document.appId)

    /** 受け取った記録を今のアプリに足す。申し出の後にアプリが切り替わっていることもあるので、ここでも判定し直す。 */
    internal suspend fun import(payload: NearbyShare.Payload): NearbyShare.Ack {
        val h = payload.header
        val from = NearbyService.label(h.sender)
        when (val d = decide(h)) {
            is NearbyShare.Decision.Reject -> {
                runtime.showToast(UiText.Raw(d.reason))
                return NearbyShare.Ack(ok = false, added = 0, message = d.reason)
            }
            is NearbyShare.Decision.Accept -> {
                val entity = runtime.entityModel(d.entityId) ?: return NearbyShare.Ack(ok = false, added = 0)
                val rows = NearbyShare.sanitize(payload.records, entity, d.sameApp)
                if (rows.isEmpty()) {
                    runtime.showToast(UiText.Res(R.string.nearby_nothing_importable_from, from))
                    return NearbyShare.Ack(ok = false, added = 0, message = service.text(UiText.Res(R.string.nearby_nothing_importable)))
                }
                var added = 0
                for (row in rows) if (runtime.createRecord(entity.id, row) != null) added++
                runtime.showToast(UiText.Plural(R.plurals.nearby_imported, added, listOf(from, entity.name, added)))
                return NearbyShare.Ack(ok = added > 0, added = added)
            }
        }
    }
}

/** 近距離共有・すれ違い通信の端末の設定。 */
class NearbySettings(private val store: KeyValueStore) {
    /** 名前を出さない(表示名の代わりに「近くの人」を送る)。iOS と同じキー。 */
    var anonymous: Boolean
        get() = store.getBoolean(ANONYMOUS_KEY) == true
        set(value) = store.putBoolean(ANONYMOUS_KEY, value)

    companion object {
        const val ANONYMOUS_KEY = "passby.anonymous"
    }
}

/** どこかの画面に送る方の `nearbyShare` があるか(container の子も見る)。passby(#425)は部品が表示中だけ自分で探す。 */
val AppDSL.usesNearbyShare: Boolean
    get() {
        fun isSend(c: Component): Boolean = c.type == ComponentType.NEARBY_SHARE && c.mode != ComponentMode.PASSBY
        fun walk(c: Component): Boolean = isSend(c) || c.children.any(::walk)
        return screens.any { s -> s.components.any(::walk) }
    }
