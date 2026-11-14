package jp.kz2601.metamon.desktop.auth

/** 端末に残す認証の情報。ID トークンは短命なので残さず、起動時に [refreshToken] から取り直す。 */
data class StoredCredential(val uid: String, val refreshToken: String) {
    /** ログに出してよい形(トークンは伏せる)。 */
    override fun toString(): String = "StoredCredential(uid=$uid)"
}

/**
 * リフレッシュトークンの置き場(#2150)。平文のファイル(`store/`)には書かない。
 * TODO(#2152): Windows の資格情報マネージャー(CredWrite / CredRead)
 * TODO(#2153): Linux の Secret Service(D-Bus)
 */
interface TokenStore {
    fun load(): StoredCredential?
    fun save(credential: StoredCredential)
    fun clear()
}

/** メモリだけの置き場(テスト・OS の置き場が使えない時)。再起動で消える。 */
class InMemoryTokenStore(initial: StoredCredential? = null) : TokenStore {
    @Volatile private var value: StoredCredential? = initial

    override fun load(): StoredCredential? = value
    override fun save(credential: StoredCredential) { value = credential }
    override fun clear() { value = null }
}
