package jp.kz2601.metamon.settings

import java.net.URI

/**
 * 設定の「詳細(サポート用)」に出す値(iOS の `SupportInfo`・#279)。音声モデル・接続先・ビルド番号を 1 画面目に出さず、ここにまとめる。
 *
 * iOS はユーザー ID も出すが、Android は出さない(#898: uid・トークン・メールアドレスは表示にもコピーにも入れない)。
 * 接続先はホスト名だけ(パス・クエリ・ユーザー情報は落とす)。
 */
data class SupportInfo(
    val voiceModel: String,
    val aiEndpoint: String,
    val appConfigEndpoint: String,
    val appVersion: String,
    val build: String,
    val appRevision: Int,
) {
    enum class Field { VOICE_MODEL, AI, APPCONFIG, VERSION, APP_REVISION }

    data class Row(val field: Field, val value: String)

    val rows: List<Row>
        get() = listOf(
            Row(Field.VOICE_MODEL, voiceModel),
            Row(Field.AI, aiEndpoint),
            Row(Field.APPCONFIG, appConfigEndpoint),
            Row(Field.VERSION, "$appVersion ($build)"),
            Row(Field.APP_REVISION, "v$appRevision"),
        )

    /** 問い合わせに貼る用。1 行 1 項目。[label] は表示と同じ見出し */
    fun clipboardText(label: (Field) -> String): String = rows.joinToString("\n") { "${label(it.field)}: ${it.value}" }

    companion object {
        /** 接続先の表示: ホスト名だけ。読めなければ "live"(iOS の `BuildConfig.aiBaseURL?.host() ?? "live"`) */
        fun host(url: String): String = runCatching { URI(url).host }.getOrNull()?.takeIf { it.isNotBlank() } ?: "live"
    }
}
