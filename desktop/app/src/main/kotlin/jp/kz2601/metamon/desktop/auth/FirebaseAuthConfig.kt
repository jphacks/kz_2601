package jp.kz2601.metamon.desktop.auth

/**
 * Firebase Auth の REST の接続先(#2150)。API キーはリポジトリに書かず、`DesktopApiConfig` と同じく環境変数から読む。
 *
 * 環境変数:
 * - `TAILORIT_FIREBASE_API_KEY` に Firebase の Web API キー(Firebase コンソールのプロジェクト設定 > 全般)。無ければサインインしない
 * - `TAILORIT_IDENTITY_TOOLKIT_URL` / `TAILORIT_SECURE_TOKEN_URL` で接続先を差し替え(Auth エミュレーターなど)
 */
data class FirebaseAuthConfig(
    val apiKey: String?,
    val identityToolkitUrl: String = IDENTITY_TOOLKIT_URL,
    val secureTokenUrl: String = SECURE_TOKEN_URL,
) {
    val isConfigured: Boolean get() = !apiKey.isNullOrEmpty()

    /** ログに出してよい形(キーは伏せる)。 */
    override fun toString(): String =
        "FirebaseAuthConfig(apiKey=${if (apiKey.isNullOrEmpty()) "none" else "***"}, identityToolkitUrl=$identityToolkitUrl, secureTokenUrl=$secureTokenUrl)"

    companion object {
        const val IDENTITY_TOOLKIT_URL = "https://identitytoolkit.googleapis.com/v1"
        const val SECURE_TOKEN_URL = "https://securetoken.googleapis.com/v1"

        fun fromEnvironment(env: Map<String, String> = System.getenv()): FirebaseAuthConfig = FirebaseAuthConfig(
            apiKey = env["TAILORIT_FIREBASE_API_KEY"]?.trim()?.takeIf { it.isNotEmpty() },
            identityToolkitUrl = env["TAILORIT_IDENTITY_TOOLKIT_URL"]?.takeIf { it.isNotBlank() } ?: IDENTITY_TOOLKIT_URL,
            secureTokenUrl = env["TAILORIT_SECURE_TOKEN_URL"]?.takeIf { it.isNotBlank() } ?: SECURE_TOKEN_URL,
        )
    }
}
