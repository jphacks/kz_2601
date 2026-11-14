package jp.kz2601.metamon.desktop.auth

/**
 * リフレッシュトークンを OS の資格情報ストアに置く口(#1749: ファイルに平文で置かない)。
 * Windows は資格情報マネージャー(#2152)、Linux は Secret Service(#2153)。
 */
interface TokenVault {
    fun save(token: String)

    /** 無ければ null。 */
    fun read(): String?

    fun delete()
}

/** 保存しない(ストアが使えない時)。次の起動でサインインし直す。[reasonKey] は設定に出す理由の文言のキー(null は理由なし)。 */
class NoTokenVault(val reasonKey: String? = null) : TokenVault {
    override fun save(token: String) = Unit

    override fun read(): String? = null

    override fun delete() = Unit
}
