package jp.kz2601.metamon.nearby

import android.util.Log
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.nearby.PassbyDistanceSupport
import jp.kz2601.metamon.renderer.components.nearby.PassbyPeer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.temporal.ChronoUnit

/**
 * `METAMON_MOCK=1` 用(iOS の `MockPassbyTransport`): Nearby Connections を使わず、少し待って「テスト花子」とすれ違ったことにする。
 * 受け取りの保存・重複除けは本物と同じ経路を通る。距離・方向は Android では測らないので出さない。
 */
class MockPassbyTransport(
    private val scope: CoroutineScope,
    private val delayMillis: Long = 1_500,
    private val now: () -> Instant = Instant::now,
) : PassbyTransport {
    override var onPayload: ((NearbyPassby.Payload) -> Unit)? = null
    override var onPeers: ((List<PassbyPeer>) -> Unit)? = null
    override var onNotice: ((String?) -> Unit)? = null
    override val distanceSupport: PassbyDistanceSupport = PassbyDistanceSupport.Unsupported()

    private var job: Job? = null

    override fun start(start: PassbyStart) {
        stop()
        job = scope.launch {
            delay(delayMillis)
            val mine = start.payload() // 送る側の組み立ても通す
            runCatching { Log.i("Passby", "mock: would send ${mine.records.size} records") }
            if (start.shouldExchange(PEER_ID)) onPayload?.invoke(payload(start, now()))
        }
    }

    override fun stop() {
        job?.cancel()
        job = null
        onPeers?.invoke(emptyList())
    }

    companion object {
        const val PEER_ID = "mock-hanako"
        const val PEER_NAME = "テスト花子"

        /** 相手の記録 1 件(許可 field に型に合う見本の値)。 */
        fun payload(start: PassbyStart, now: Instant = Instant.now()): NearbyPassby.Payload {
            val data = mutableMapOf<String, JSONValue>()
            val today = now.atZone(ZoneId.systemDefault())
            NearbyPassby.allowedFields(start.config, start.entity).forEachIndexed { i, f ->
                val v: JSONValue? = when (f.type) {
                    FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY -> JSONValue.Num(20.0)
                    FieldType.BOOLEAN -> JSONValue.Bool(true)
                    FieldType.DATE -> JSONValue.Str(today.toLocalDate().toString())
                    FieldType.DATETIME -> JSONValue.Str(now.truncatedTo(ChronoUnit.SECONDS).toString())
                    FieldType.ENUM -> f.options?.firstOrNull()?.let { JSONValue.Str(it) }
                    FieldType.LOCATION -> JSONValue.Str("35.6812,139.7671")
                    else -> JSONValue.Str(if (i == 0) PEER_NAME else "よろしくお願いします")
                }
                if (v != null) data[f.id] = v
            }
            return NearbyPassby.Payload(NearbyPassby.VERSION, PEER_ID, PEER_NAME, start.entity.id, listOf(NearbyPassby.Item("mock-1", data)))
        }
    }
}
