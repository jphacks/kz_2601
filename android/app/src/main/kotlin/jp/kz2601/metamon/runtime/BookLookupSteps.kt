package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.logic.BookLookupPlan
import jp.kz2601.metamon.dsl.logic.fields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.services.books.BookLookupClient
import jp.kz2601.metamon.services.books.isBookLookupOff
import jp.kz2601.metamon.services.http.ApiError
import kotlinx.coroutines.CancellationException

/**
 * `lookupBook`(iOS / Mac の `BookLookupSteps`): フォームの ISBN から本の情報を引き、書名などの field を上書きする(保存はしない)。
 * 見つからない・つながらない時はメッセージだけで手入力に戻る。
 */
object BookLookupSteps {
    val FORM_ONLY = UiText.Res(R.string.host_book_form_only)
    val ISBN_EMPTY = UiText.Res(R.string.host_book_isbn_empty)
    val ISBN_INVALID = UiText.Res(R.string.host_book_isbn_invalid)
    val UNAVAILABLE = UiText.Res(R.string.host_book_unavailable)
    val NOT_FOUND = UiText.Res(R.string.host_book_not_found)
    val OFF = UiText.Res(R.string.host_book_off)

    /** 引いてフォームに入れる。戻り値は toast の一言(待つ間にフォームが変わって何もしなかった時は null)。 */
    suspend fun run(action: Action, form: FormDraft?, client: BookLookupClient?): UiText? {
        val b = action.bookLookup
        if (b == null || form == null || form.isReadOnly) return FORM_ONLY
        val spec = b.fields
        val raw = (form.values[spec.isbnField] ?: JSONValue.Null).displayString
        if (raw.isBlank()) return ISBN_EMPTY
        val isbn = BookLookupPlan.normalizeISBN(raw) ?: return ISBN_INVALID
        client ?: return UNAVAILABLE
        // 待つ間に別の記録へ移った・ISBN を打ち直した・欄を手で直した時に、古い結果で上書きしない
        val recordId = form.recordId
        val before = form.values.toMap()
        return try {
            val book = client.lookup(isbn)
            if (form.isReadOnly || form.recordId != recordId || value(form.values, spec.isbnField) != value(before, spec.isbnField)) return null
            for ((id, v) in BookLookupPlan.values(spec, book, form.entity)) {
                if (value(form.values, id) == value(before, id)) form.values[id] = v
            }
            form.values[spec.isbnField] = JSONValue.Str(isbn)
            UiText.Res(R.string.host_book_filled, book.title, book.source.name)
        } catch (e: CancellationException) {
            throw e
        } catch (e: ApiError) {
            when {
                e.status == 404 -> NOT_FOUND
                e.isBookLookupOff -> OFF
                e.status == 400 -> ISBN_INVALID
                else -> UNAVAILABLE
            }
        } catch (_: Exception) {
            UNAVAILABLE
        }
    }

    private fun value(values: Map<String, JSONValue>, id: String): JSONValue = values[id] ?: JSONValue.Null
}
