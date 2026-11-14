package jp.kz2601.metamon.nearby

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyShare
import jp.kz2601.metamon.runtime.UiText
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * `nearbyShare`(#36・#1382)の送受信。iOS の `NearbyService` と同じ動き。通り道は [NearbyTransport]。
 *
 * - 受信: 前面にいる間だけ待ち受ける。つながった相手から届いた Header を見て確認を出し、承認した時だけ READY を返して受け取る
 * - 送信: シートを開いている間だけ探す。相手を選ぶとつないで Header を送り、READY が来たら Payload を 1 回送って Ack を待つ(時間切れあり)
 * - 1 回の受け渡しが終わったら切る。呼ぶのはメインスレッドだけ
 */
class NearbyService(
    private val transport: NearbyTransport,
    private val scope: CoroutineScope,
    displayName: String,
    private val timeoutMillis: Long = TIMEOUT_MILLIS,
) : NearbyTransport.Listener {

    data class Peer(val id: String, val name: String)

    sealed interface SendState {
        data object Idle : SendState
        data class Inviting(val name: String) : SendState
        data class Sending(val name: String) : SendState
        data class Done(val message: UiText) : SendState
        data class Failed(val message: UiText) : SendState

        val isBusy: Boolean get() = this is Inviting || this is Sending
    }

    /** 承認待ちの申し出(確認ダイアログを出す)。 */
    data class Incoming(val endpointId: String, val header: NearbyShare.Header, val decision: NearbyShare.Decision)

    private val _peers = MutableStateFlow<List<Peer>>(emptyList())
    val peers: StateFlow<List<Peer>> = _peers.asStateFlow()
    private val _sendState = MutableStateFlow<SendState>(SendState.Idle)
    val sendState: StateFlow<SendState> = _sendState.asStateFlow()
    private val _incoming = MutableStateFlow<Incoming?>(null)
    val incoming: StateFlow<Incoming?> = _incoming.asStateFlow()

    /** 相手に見える名前(端末の名前と Header の送り主)。「名前を出さない」なら [NearbyLimits.ANONYMOUS_NAME]。 */
    private val _myName = MutableStateFlow(NearbyLimits.peerName(displayName, anonymous = false))
    val myName: StateFlow<String> = _myName.asStateFlow()

    /** 届いた申し出の取り込み判定(今開いているアプリで判定)。 */
    var decide: ((NearbyShare.Header) -> NearbyShare.Decision)? = null

    /** 受け取ったデータを取り込む。戻り値は送信側に返す Ack(文は受け取った側の言語)。 */
    var onReceive: (suspend (NearbyShare.Payload, NearbyShare.Decision) -> NearbyShare.Ack)? = null

    /** 受け取れなかった時に送り返す文(画面の言語で解く)。 */
    var text: (UiText) -> String = { (it as? UiText.Raw)?.text.orEmpty() }

    private var advertising = false
    private var browsing = false

    private class Outgoing(val endpointId: String, val name: String, val header: ByteArray, val payload: ByteArray) {
        val reader = NearbyWire.Reader()
        var connected = false
    }

    private var outgoing: Outgoing? = null
    private var timeoutJob: Job? = null

    /** 受信中の相手 → 読みかけの切れと、承認した判定。 */
    private class Inbound {
        val reader = NearbyWire.Reader()
        var decision: NearbyShare.Decision? = null
        var asked = false
    }

    private val inbound = mutableMapOf<String, Inbound>()

    init {
        transport.listener = this
    }

    /** 相手に見える名前を変える(「名前を出さない」の切り替え)。待ち受け中なら、途中の受け渡しを切ってから新しい名前でやり直す。 */
    fun rename(displayName: String) {
        val name = NearbyLimits.peerName(displayName, anonymous = false)
        if (name == _myName.value) return
        val wasAdvertising = advertising
        val wasBrowsing = browsing
        setAdvertising(false)
        if (wasBrowsing) stopBrowsing()
        transport.stopAll()
        inbound.clear()
        _myName.value = name
        if (wasAdvertising) setAdvertising(true)
        if (wasBrowsing) startBrowsing()
    }

    // MARK: - 受信(待ち受け)

    fun setAdvertising(on: Boolean) {
        if (on == advertising) return
        advertising = on
        if (on) {
            transport.startAdvertising(_myName.value)
        } else {
            transport.stopAdvertising()
            _incoming.value?.let { answer(it, accept = false) }
        }
    }

    /** 確認ダイアログの結果。拒否・取り込めない時は切る(勝手に受け取らない)。 */
    fun answer(item: Incoming, accept: Boolean) {
        if (_incoming.value == item) _incoming.value = null
        val ok = accept && item.decision is NearbyShare.Decision.Accept
        val state = inbound[item.endpointId]
        if (ok && state != null) {
            state.decision = item.decision
            sendMessage(item.endpointId, NearbyWire.Kind.READY, ByteArray(0))
        } else {
            drop(item.endpointId)
        }
    }

    private fun onHeader(endpointId: String, state: Inbound, body: ByteArray) {
        if (state.asked) return drop(endpointId)
        state.asked = true
        // Header が読めない・上限を超える申し出と、確認中の二重の申し出は黙って切る
        val header = NearbyShare.decodeHeader(body)
        if (header == null || _incoming.value != null || !advertising) return drop(endpointId)
        val decision = decide?.invoke(header) ?: NearbyShare.Decision.Reject(text(UiText.Res(R.string.nearby_cannot_receive_now)))
        // 曲のリクエスト(#412)は確認を出さずに受ける。受けられないアプリなら黙って切る
        if (header.song != null) {
            if (decision !is NearbyShare.Decision.Accept) return drop(endpointId)
            state.decision = decision
            sendMessage(endpointId, NearbyWire.Kind.READY, ByteArray(0))
            return
        }
        _incoming.value = Incoming(endpointId, header, decision)
    }

    private fun onPayload(endpointId: String, state: Inbound, body: ByteArray) {
        val decision = state.decision ?: return drop(endpointId) // 承認していない相手からは取り込まない
        state.decision = null
        scope.launch {
            // 大きさ・名前・id の長さを読む前に確かめる(上限を超えたら取り込まない)
            val payload = NearbyShare.decodePayload(body)
            val handler = onReceive
            val ack = if (payload != null && handler != null) {
                handler(payload, decision)
            } else {
                NearbyShare.Ack(ok = false, added = 0, message = text(UiText.Res(R.string.nearby_unreadable)))
            }
            sendMessage(endpointId, NearbyWire.Kind.ACK, NearbyShare.encode(ack))
        }
    }

    private fun drop(endpointId: String) {
        inbound.remove(endpointId)
        transport.disconnect(endpointId)
    }

    // MARK: - 送信(探す)

    fun startBrowsing() {
        _peers.value = emptyList()
        _sendState.value = SendState.Idle
        if (browsing) return
        browsing = true
        transport.startDiscovery()
    }

    fun stopBrowsing() {
        if (browsing) transport.stopDiscovery()
        browsing = false
        _peers.value = emptyList()
        if (_sendState.value.isBusy) finishOutgoing(SendState.Idle)
    }

    fun send(payload: NearbyShare.Payload, peer: Peer) {
        if (_sendState.value.isBusy || !browsing) return
        val sent = payload.sentAs(_myName.value) // 送り主は今の端末の名前に揃える
        val out = Outgoing(peer.id, peer.name, NearbyShare.encode(sent.header), NearbyShare.encode(sent))
        if (out.header.size > NearbyShare.MAX_HEADER_BYTES || out.payload.size > NearbyShare.MAX_SEND_BYTES) {
            _sendState.value = SendState.Failed(UiText.Res(R.string.nearby_build_failed))
            return
        }
        outgoing = out
        _sendState.value = SendState.Inviting(peer.name)
        timeoutJob?.cancel()
        timeoutJob = scope.launch {
            delay(timeoutMillis)
            if (outgoing === out && _sendState.value.isBusy) finishOutgoing(SendState.Failed(UiText.Res(R.string.nearby_no_response, label(peer.name))))
        }
        transport.connect(peer.id, _myName.value)
    }

    private fun finishOutgoing(state: SendState) {
        timeoutJob?.cancel()
        timeoutJob = null
        val out = outgoing
        outgoing = null
        _sendState.value = state
        out?.let { transport.disconnect(it.endpointId) }
    }

    private fun onOutgoingMessage(out: Outgoing, message: NearbyWire.Message) {
        when (message.kind) {
            NearbyWire.Kind.READY -> if (_sendState.value is SendState.Inviting) {
                _sendState.value = SendState.Sending(out.name)
                sendMessage(out.endpointId, NearbyWire.Kind.PAYLOAD, out.payload)
            }
            NearbyWire.Kind.ACK -> {
                val ack = NearbyShare.decodeAck(message.body)
                    ?: return finishOutgoing(SendState.Failed(UiText.Res(R.string.nearby_not_received, label(out.name))))
                finishOutgoing(
                    if (ack.ok) {
                        SendState.Done(UiText.Plural(R.plurals.nearby_delivered, ack.added, listOf(label(out.name), ack.added)))
                    } else {
                        SendState.Failed(ack.message?.let { UiText.Raw(it) } ?: UiText.Res(R.string.nearby_not_received, label(out.name)))
                    },
                )
            }
            else -> finishOutgoing(SendState.Failed(UiText.Res(R.string.nearby_send_failed)))
        }
    }

    private fun sendMessage(endpointId: String, kind: NearbyWire.Kind, body: ByteArray) {
        NearbyWire.frames(kind, body).forEach { transport.send(endpointId, it) }
    }

    // MARK: - 通り道の知らせ

    override fun onFound(endpointId: String, name: String) {
        if (!browsing || _peers.value.any { it.id == endpointId }) return
        _peers.value = _peers.value + Peer(endpointId, name)
    }

    override fun onLost(endpointId: String) {
        _peers.value = _peers.value.filter { it.id != endpointId }
    }

    override fun onConnected(endpointId: String, name: String, outgoing: Boolean) {
        val out = this.outgoing
        if (outgoing) {
            if (out == null || out.endpointId != endpointId) return transport.disconnect(endpointId)
            out.connected = true
            sendMessage(endpointId, NearbyWire.Kind.HEADER, out.header)
            return
        }
        // 受ける側: 待ち受けていない・送っている最中ならつながらない
        if (!advertising || _sendState.value.isBusy) return transport.disconnect(endpointId)
        inbound[endpointId] = Inbound()
    }

    override fun onDisconnected(endpointId: String) {
        inbound.remove(endpointId)
        if (_incoming.value?.endpointId == endpointId) _incoming.value = null
        val out = outgoing
        if (out != null && out.endpointId == endpointId && _sendState.value.isBusy) {
            val declined = _sendState.value is SendState.Inviting
            finishOutgoing(
                SendState.Failed(if (declined) UiText.Res(R.string.nearby_declined, label(out.name)) else UiText.Res(R.string.nearby_disconnected)),
            )
        }
    }

    override fun onFrame(endpointId: String, frame: ByteArray) {
        val out = outgoing
        if (out != null && out.endpointId == endpointId) {
            val message = out.reader.accept(frame)
            if (out.reader.broken) return finishOutgoing(SendState.Failed(UiText.Res(R.string.nearby_send_failed)))
            message?.let { onOutgoingMessage(out, it) }
            return
        }
        val state = inbound[endpointId] ?: return
        val message = state.reader.accept(frame)
        if (state.reader.broken) return drop(endpointId)
        when (message?.kind) {
            null -> Unit
            NearbyWire.Kind.HEADER -> onHeader(endpointId, state, message.body)
            NearbyWire.Kind.PAYLOAD -> onPayload(endpointId, state, message.body)
            else -> drop(endpointId)
        }
    }

    override fun onError(advertising: Boolean) {
        if (advertising) {
            this.advertising = false
        } else {
            browsing = false
            _sendState.value = SendState.Failed(UiText.Res(R.string.nearby_discovery_failed))
        }
    }

    /** 全部やめる(アプリを閉じる時)。 */
    fun close() {
        setAdvertising(false)
        stopBrowsing()
        transport.stopAll()
        inbound.clear()
    }

    companion object {
        /** 申し出から Ack までの時間切れ(iOS の 35 秒)。 */
        const val TIMEOUT_MILLIS = 35_000L

        /** 画面に出す相手の名前。匿名の印(「近くの人」)は今の言語で出す。 */
        fun label(name: String): Any {
            val n = NearbyLimits.displayName(name)
            return if (n == NearbyLimits.ANONYMOUS_NAME) UiText.Res(R.string.nearby_anonymous) else n
        }
    }
}
