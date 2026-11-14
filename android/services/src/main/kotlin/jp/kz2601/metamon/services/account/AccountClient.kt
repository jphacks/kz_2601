package jp.kz2601.metamon.services.account

import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp

/**
 * アカウント削除(#841・iOS の `AccountClient`)。Play のポリシーでアプリ内の削除が要る。
 * 実装: [HttpAccountClient](`DELETE /v1/me`)/ [MockAccountClient](何もしない)。
 */
interface AccountClient {
    /** サーバーでアカウントを論理削除する(アプリは archived・シェア取り消し・掲載取り下げ。以後のリクエストは 410)。冪等。 */
    suspend fun deleteAccount()
}

/** `DELETE /v1/me`(docs/backend/openapi-appconfig.yaml)。成功は 204。 */
class HttpAccountClient(private val http: MetamonHttp) : AccountClient {
    override suspend fun deleteAccount() {
        // アプリ・シェア・掲載の更新でサーバー側が数秒かかることがある
        http.send(HttpRequest("DELETE", listOf("v1", "me"), timeoutMillis = 60_000))
    }
}

/** モックの構成用。サーバーに何も無いので成功だけ返す。 */
object MockAccountClient : AccountClient {
    override suspend fun deleteAccount() = Unit
}
