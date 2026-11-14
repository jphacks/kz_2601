package jp.kz2601.metamon.services.http

/** SSE の 1 件(`event:` と `data:` を空行で区切ったもの)。 */
data class SseFrame(val event: String, val data: String)

/**
 * SSE の行を組み立てる(iOS の `SSEParser`)。`:` で始まる行(keep-alive)と `id:` / `retry:` は読み飛ばす。
 * 区切りの空行が来るまでは 1 件にしない(途中で切れた最後の 1 件は出さない)。
 */
class SseParser {
    private val pending = StringBuilder()
    private var event = ""
    private val data = mutableListOf<String>()

    /** 文字列の断片を流し込み、揃った分を返す(断片の境目は行の途中でもよい)。 */
    fun feed(chunk: String): List<SseFrame> {
        val out = mutableListOf<SseFrame>()
        for (c in chunk) {
            if (c == '\n') {
                feedLine(pending.toString())?.let(out::add)
                pending.setLength(0)
            } else {
                pending.append(c)
            }
        }
        return out
    }

    /** 1 行(改行を除いたもの)を流し込む。区切りの空行で 1 件を返す。 */
    fun feedLine(raw: String): SseFrame? {
        val line = raw.removeSuffix("\r")
        if (line.isEmpty()) {
            val frame = if (data.isEmpty()) null else SseFrame(event.ifEmpty { "message" }, data.joinToString("\n"))
            event = ""
            data.clear()
            return frame
        }
        if (line.startsWith(":")) return null
        val colon = line.indexOf(':')
        val field = if (colon >= 0) line.substring(0, colon) else line
        val value = if (colon >= 0) line.substring(colon + 1).removePrefix(" ") else ""
        when (field) {
            "event" -> event = value
            "data" -> data.add(value)
        }
        return null
    }
}
