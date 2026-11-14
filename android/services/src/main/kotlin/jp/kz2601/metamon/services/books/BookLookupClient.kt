package jp.kz2601.metamon.services.books

import jp.kz2601.metamon.dsl.logic.BookInfo
import jp.kz2601.metamon.services.http.ApiError

/** ISBN から本の情報を引く(DSL の lookupBook・`POST /v1/lookup/book`)。iOS の `BookLookupClient`。端末は appconfig にだけ聞く。 */
fun interface BookLookupClient {
    suspend fun lookup(isbn: String): BookInfo
}

/** 本の情報の取得を止めている 503(`details.reason = disabled`)。つながらない 503 とは分けて「今使えません」と出す。 */
val ApiError.isBookLookupOff: Boolean get() = status == 503 && reason == "disabled"
