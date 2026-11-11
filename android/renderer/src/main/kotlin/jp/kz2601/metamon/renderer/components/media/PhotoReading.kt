package jp.kz2601.metamon.renderer.components.media

import jp.kz2601.metamon.dsl.PhotoAnalyze
import jp.kz2601.metamon.dsl.PhotoAnalyzeKind
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlin.math.roundToLong

/**
 * `photoField.analyze`(iOS の `PhotoReader`)。iOS は端末内(Vision)で読み、写真を端末の外に出さない・課金しない。
 * Android も同じく端末内で読む口だけを持つ。実装(ML Kit など)は :app が [jp.kz2601.metamon.renderer.DSLRuntime.photoReader]
 * で渡す。無ければ analyze は無視して写真の入力として動く。
 */
interface PhotoReader {
    /** JPEG 1 枚から、[specs] が要る手がかりだけを取り出す。 */
    suspend fun clues(jpeg: ByteArray, specs: List<PhotoAnalyze>): PhotoClues
}

/** 1 枚の写真から取り出した手がかり(iOS の `PhotoClues` のうち、いま値にできるもの)。 */
data class PhotoClues(
    /** 文字認識の行(上から) */
    val lines: List<String> = emptyList(),
    /** バーコード・QR の中身(見つかった順) */
    val barcodes: List<String> = emptyList(),
    val peopleCount: Int? = null,
    /** 主な色の名前 */
    val color: String? = null,
)

/** 読み取りの結果。values は into の field id → 保存する形の値。 */
data class PhotoReadOutcome(val values: Map<String, JSONValue> = emptyMap(), val missed: List<String> = emptyList())

object PhotoReading {
    /** いま値にできる kind。amount / date / classify / describe / jointAngle / label / labels は子 Issue(読めなかった扱い)。 */
    val supportedKinds = setOf(PhotoAnalyzeKind.TEXT, PhotoAnalyzeKind.BARCODE, PhotoAnalyzeKind.PEOPLE_COUNT, PhotoAnalyzeKind.COLOR)

    /** 手がかり → 各 into の値。読めなかった・型に入らない項目は missed(今の値のまま)。 */
    fun apply(clues: PhotoClues, specs: List<PhotoAnalyze>, entity: Entity): PhotoReadOutcome {
        val values = linkedMapOf<String, JSONValue>()
        val missed = mutableListOf<String>()
        for (spec in specs) {
            val type = entity.field(spec.into)?.type
            val v = type?.let { jsonValue(reading(spec, clues), it) }
            if (v == null) missed += spec.into else values[spec.into] = v
        }
        return PhotoReadOutcome(values, missed)
    }

    private fun reading(spec: PhotoAnalyze, clues: PhotoClues): Any? = when (spec.kind) {
        PhotoAnalyzeKind.TEXT -> clues.lines.joinToString("\n").takeIf { it.isNotEmpty() }
        PhotoAnalyzeKind.BARCODE -> clues.barcodes.firstOrNull()
        PhotoAnalyzeKind.PEOPLE_COUNT -> clues.peopleCount?.toDouble()
        PhotoAnalyzeKind.COLOR -> clues.color
        else -> null
    }

    /** 読んだ値を into の型の保存形式に(iOS の `PhotoReadingValue.jsonValue`)。入らない組み合わせは null。 */
    fun jsonValue(reading: Any?, type: FieldType): JSONValue? = when {
        reading is String && type in setOf(FieldType.STRING, FieldType.TEXT, FieldType.ENUM) ->
            reading.trim().takeIf { it.isNotEmpty() }?.let { JSONValue.Str(it) }
        reading is Double && (type == FieldType.INTEGER || type == FieldType.CURRENCY) -> JSONValue.Num(reading.roundToLong().toDouble())
        reading is Double && type == FieldType.NUMBER -> JSONValue.Num((reading * 10).roundToLong() / 10.0)
        else -> null
    }
}
