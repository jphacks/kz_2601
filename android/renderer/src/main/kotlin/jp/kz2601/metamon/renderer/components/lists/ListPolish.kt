package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.runtime.staticCompositionLocalOf
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType

/** 描いている画面の DSL(空の一覧の「追加」が同じ画面の追加ボタンを探す)。ScreenView が渡す。 */
val LocalAppDsl = staticCompositionLocalOf<AppDSL?> { null }

/** 保存で増えた行を光らせる(iOS の `ListLeaf.markNewRows`・#1118)。 */
object NewRowHighlight {
    const val MILLIS = 1_200L

    /**
     * 前回描いた id に無い先頭の行。初回の描画・空からの初回ロード・動きを減らす設定では光らせない(null)。
     */
    fun fresh(seen: Set<String>?, now: List<String>, reduceMotion: Boolean): String? {
        if (seen.isNullOrEmpty() || reduceMotion) return null
        return now.firstOrNull { it !in seen }
    }
}

/** 空の一覧の「最初の 1 件を追加しましょう」の出し方(iOS の `EmptyPlaceholder.cta`)。 */
enum class EmptyCta {
    NONE,

    /** 同じ画面の追加と同じフォームを開くボタン */
    BUTTON,

    /** 追加の手段が無い画面: ブランド色の文字だけ */
    TEXT,
    ;

    companion object {
        /** 本当に空の時だけ(読み込み中・取得失敗では出さない)。 */
        fun of(kind: RecordsPlaceholder, canAdd: Boolean): EmptyCta = when {
            kind != RecordsPlaceholder.EMPTY -> NONE
            canAdd -> BUTTON
            else -> TEXT
        }
    }
}

/** stat の左のアイコン(iOS の `StatLeaf.iconName` / `FieldIcon`)。 */
object StatIcon {
    /** 指定があればそれ。計算式モードで未指定なら出さない(#193)。他は集計と項目の型から。 */
    fun name(c: Component, field: Field?): String? =
        c.icon ?: if (StatValue.isValueMode(c)) null else forAggregate(c.aggregate?.wire, field)

    fun forAggregate(aggregate: String?, field: Field?): String = when (aggregate) {
        "count" -> "number"
        "avg" -> "chart.bar"
        "max" -> "arrow.up.right"
        "min" -> "arrow.down.right"
        "distance" -> "point.bottomleft.forward.to.point.topright.scurvepath"
        "duration" -> "clock"
        else -> infer(field, field?.name.orEmpty())
    }

    /** 項目の型(と id / ラベルの語)から(iOS の `FieldIcon.infer(field:hint:)`)。 */
    fun infer(field: Field?, hint: String = ""): String {
        val h = hint.lowercase()
        when (field?.type) {
            FieldType.CURRENCY -> return "yensign"
            FieldType.DATE -> return "calendar"
            FieldType.DATETIME -> return if ("time" in h || "時" in h) "clock" else "calendar.badge.clock"
            FieldType.INTEGER, FieldType.NUMBER -> return when {
                "weight" in h || "重" in h -> "scalemass"
                "rep" in h || "回" in h -> "repeat"
                "step" in h || "歩" in h -> "figure.walk"
                else -> "number"
            }
            FieldType.ENUM -> return "list.bullet"
            FieldType.PHOTO -> return "camera"
            FieldType.VIDEO -> return "video"
            FieldType.AUDIO -> return "waveform"
            FieldType.LOCATION -> return "mappin.and.ellipse"
            FieldType.BOOLEAN -> return "checkmark.circle"
            FieldType.TEXT -> return "note.text"
            else -> Unit
        }
        return when {
            listOf("memo", "note", "comment", "メモ", "ひとこと").any { it in h } -> "note.text"
            listOf("pin", "password", "pass", "ロック", "暗証").any { it in h } -> "lock"
            listOf("mail", "メール").any { it in h } -> "envelope"
            listOf("url", "link", "リンク").any { it in h } -> "link"
            listOf("phone", "tel", "電話").any { it in h } -> "phone"
            listOf("name", "title", "名").any { it in h } -> "textformat"
            listOf("search", "検索").any { it in h } -> "magnifyingglass"
            else -> "pencil.line"
        }
    }
}
