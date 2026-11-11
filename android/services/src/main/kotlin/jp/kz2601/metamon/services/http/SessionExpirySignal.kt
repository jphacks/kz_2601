package jp.kz2601.metamon.services.http

/**
 * 「トークンを取り直しても 401」の知らせ(#1102)。[MetamonHttp] はサインインの部品より先に組み立てるので、
 * 受け手(サインアウトしてサインイン画面に戻す)は後から [listener] に入れる。
 */
class SessionExpirySignal {
    @Volatile
    var listener: (() -> Unit)? = null

    fun fire() {
        listener?.invoke()
    }
}
