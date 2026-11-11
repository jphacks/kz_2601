package jp.kz2601.metamon.services.http

/**
 * 「トークンを取り直しても 401」の知らせ(#1102)。[MetamonHttp] はサインインの部品より先に組み立てるので、
 * 受け手(サインアウトしてサインイン画面に戻す)は後から [listener] に入れる。
 * 値は送った時の uid([TokenProvider.subject])。受け手は今の人と違えば捨てる(別の人に替わった後に届いた 401)。
 */
class SessionExpirySignal {
    @Volatile
    var listener: ((String?) -> Unit)? = null

    fun fire(uid: String?) {
        listener?.invoke(uid)
    }
}
