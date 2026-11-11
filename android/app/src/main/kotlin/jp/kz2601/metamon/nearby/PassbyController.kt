package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.renderer.components.nearby.NearbyPassbyController
import jp.kz2601.metamon.renderer.components.nearby.PassbyState
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.util.UUID

/**
 * すれ違い通信(#1383・iOS の `PassbyController`)の本体側。`NearbyPassbyLeaf` のスイッチ・同意・会った人の一覧と、
 * 通り道([PassbyTransport])・記録の保存を結ぶ。呼ぶのはメインスレッドだけ。
 *
 * 端末に保存するもの([KeyValueStore]・サーバーへは送らない。キーは iOS と同じ):
 * - 同意した版 / 名前を出さないか(近くの人に送ると同じ [NearbySettings])/ 部品ごとのオン・オフ / この端末の senderId(ランダム)
 * - アプリごとの台帳(受け取った記録の内容・会った人)
 */
class PassbyController(
    private val runtime: AppRuntime,
    private val transport: PassbyTransport,
    private val settings: NearbySettings,
    private val scope: CoroutineScope,
    /** 自分の表示名(投稿の名前 → 端末の名前) */
    private val displayName: () -> String,
    private val now: () -> Instant = Instant::now,
) : NearbyPassbyController {
    private val store: KeyValueStore = settings.store

    private val _state = MutableStateFlow(
        PassbyState(
            consentedVersion = store.getInt(CONSENT_KEY),
            isAnonymous = settings.anonymous,
            displayName = displayName(),
            distanceSupport = transport.distanceSupport,
        ),
    )
    override val state: StateFlow<PassbyState> = _state.asStateFlow()

    private var current: NearbyPassby.Config? = null
    private var ledger = NearbyPassby.Ledger()
    private var ledgerAppId: String? = null

    init {
        transport.onPayload = { p -> scope.launch { receive(p) } }
        transport.onPeers = { peers -> _state.update { it.copy(peers = peers) } }
        transport.onNotice = { notice -> _state.update { it.copy(notice = notice) } }
        // 近くの人に送るシートで切り替えても、名前を変えて探し直す
        settings.onAnonymousChanged { on ->
            _state.update { it.copy(isAnonymous = on) }
            if (_state.value.isAdvertising) restart()
        }
        runtime.passby = this
    }

    private val appId: String get() = runtime.document.appId

    private val senderId: String
        get() = store.getString(SENDER_ID_KEY) ?: UUID.randomUUID().toString().lowercase().also { store.putString(SENDER_ID_KEY, it) }

    // NearbyPassbyController

    override fun isOptedIn(config: NearbyPassby.Config): Boolean =
        config.componentId in _state.value.optedIn || store.getBoolean(optInKey(appId, config.componentId)) == true

    override fun setOptIn(on: Boolean, config: NearbyPassby.Config) {
        store.putBoolean(optInKey(appId, config.componentId), on)
        _state.update { it.copy(optedIn = if (on) it.optedIn + config.componentId else it.optedIn - config.componentId) }
        if (current?.componentId == config.componentId) restart()
    }

    override fun agree(anonymous: Boolean) {
        store.putInt(CONSENT_KEY, NearbyPassby.CONSENT_VERSION)
        _state.update { it.copy(consentedVersion = NearbyPassby.CONSENT_VERSION) }
        setAnonymous(anonymous)
    }

    /** 名前を出さない(表示名の代わりに「近くの人」を送る)。探している最中なら名前を変えて探し直す。 */
    override fun setAnonymous(on: Boolean) {
        settings.anonymous = on
    }

    override fun attach(config: NearbyPassby.Config) {
        current = config
        loadLedger()
        val opted = store.getBoolean(optInKey(appId, config.componentId)) == true
        _state.update {
            it.copy(
                displayName = displayName(),
                optedIn = if (opted) it.optedIn + config.componentId else it.optedIn,
            )
        }
        scope.launch {
            runtime.load(setOf(config.entityId)) // 送る記録を読んでおく
            if (current == config) restart()
        }
    }

    override fun detach(config: NearbyPassby.Config) {
        if (current?.componentId != config.componentId) return
        current = null
        stop()
    }

    // 開始・停止

    private fun restart() {
        val config = current
        val entity = config?.let { runtime.entityModel(it.entityId) }
        if (config == null || entity == null || !isOptedIn(config) ||
            NearbyPassby.toggle(on = true, consentedVersion = _state.value.consentedVersion) != NearbyPassby.ToggleOutcome.START
        ) {
            stop()
            return
        }
        val me = senderId
        val name = NearbyLimits.publicName(displayName(), settings.anonymous)
        val hello = NearbyPassby.Hello(app = NearbyPassby.appTag(appId), senderId = me, name = name)
        transport.start(
            PassbyStart(
                hello = hello, config = config, entity = entity,
                payload = {
                    NearbyPassby.makePayload(me, name, entity, config, runtime.records(entity.id), ledger, runtime.currentUid())
                },
                shouldExchange = { sender -> ledger.shouldExchange(sender, now()) },
            ),
        )
        _state.update { it.copy(isAdvertising = true, displayName = displayName()) }
    }

    private fun stop() {
        transport.stop()
        _state.update { it.copy(isAdvertising = false, peers = emptyList()) }
    }

    // 受け取り

    suspend fun receive(payload: NearbyPassby.Payload) {
        val config = current ?: return
        val entity = runtime.entityModel(config.entityId) ?: return
        val items = NearbyPassby.sanitize(payload, entity, config)
        var changed = 0
        for (plan in ledger.plan(items, payload.senderId)) {
            when (plan) {
                is NearbyPassby.Plan.Create -> if (runtime.createRecord(entity.id, plan.data) != null) {
                    ledger.commit(plan.key, plan.data)
                    changed++
                }
                is NearbyPassby.Plan.Update -> {
                    // 仮 id は同期で変わるのでローカル DB から読み直して内容で探す。消された記録は作り直さない(内容だけ覚え直す)
                    runtime.reloadLocal(entity.id)
                    val rec = NearbyPassby.Ledger.match(plan.old, runtime.records(entity.id))
                    if (rec != null && runtime.updateRecord(entity.id, rec.id, plan.new)) changed++
                    ledger.commit(plan.key, plan.new)
                }
                is NearbyPassby.Plan.Skip -> Unit
            }
        }
        ledger.met(payload.senderId, payload.name, now(), changed)
        saveLedger()
        if (changed > 0) {
            runtime.showToast(UiText.Plural(R.plurals.passby_met, changed, listOf(NearbyService.label(payload.name), entity.name, changed)))
        }
    }

    // 台帳

    private fun loadLedger() {
        val id = appId
        if (ledgerAppId == id) return
        ledgerAppId = id
        ledger = store.getString(ledgerKey(id))
            ?.let { runCatching { NearbyPassby.decodeLedger(it.toByteArray(Charsets.UTF_8)) }.getOrNull() }
            ?: NearbyPassby.Ledger()
        _state.update { it.copy(encounters = ledger.encounters.toList(), optedIn = emptySet()) }
    }

    private fun saveLedger() {
        _state.update { it.copy(encounters = ledger.encounters.toList()) }
        val id = ledgerAppId ?: return
        store.putString(ledgerKey(id), NearbyPassby.encode(ledger).toString(Charsets.UTF_8))
    }

    companion object {
        const val CONSENT_KEY = "passby.consent"
        const val SENDER_ID_KEY = "passby.senderId"
        fun optInKey(appId: String, component: String) = "passby.optin.$appId.$component"
        fun ledgerKey(appId: String) = "passby.ledger.$appId"
    }
}
