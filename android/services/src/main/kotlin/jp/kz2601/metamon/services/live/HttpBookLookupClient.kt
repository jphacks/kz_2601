package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.logic.BookInfo
import jp.kz2601.metamon.services.books.BookLookupClient
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.Serializable

/** appconfig の `POST /v1/lookup/book`(iOS の `HTTPBookLookupClient`)。POST なので入力された ISBN を URL に載せない。 */
class HttpBookLookupClient(private val http: MetamonHttp) : BookLookupClient {
    override suspend fun lookup(isbn: String): BookInfo {
        val body = ApiJson.encodeToString(Body.serializer(), Body(isbn))
        val res = http.send(HttpRequest("POST", listOf("v1", "lookup", "book"), body = body, timeoutMillis = TIMEOUT))
        return ApiJson.decodeFromString(Book.serializer(), res).toInfo()
    }

    @Serializable
    private data class Body(val isbn: String)

    @Serializable
    private data class Book(
        val isbn: String,
        val title: String,
        val authors: String? = null,
        val publisher: String? = null,
        val publishedDate: String? = null,
        val coverUrl: String? = null,
        val source: Source,
    ) {
        fun toInfo() = BookInfo(isbn, title, authors, publisher, publishedDate, coverUrl, BookInfo.Source(source.name, source.url, source.license))
    }

    @Serializable
    private data class Source(val name: String, val url: String, val license: String? = null)

    private companion object {
        const val TIMEOUT = 20_000L
    }
}
