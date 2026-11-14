package jp.kz2601.metamon.desktop.auth

import java.net.URLEncoder
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

/** 1 回のサインインで使う PKCE の値。ログに出さないよう [toString] は値を伏せる。 */
class PkceRequest(
    val codeVerifier: String,
    val codeChallenge: String,
    val state: String,
) {
    override fun toString(): String = "PkceRequest(method=S256)"
}

/**
 * Google のサインイン(127.0.0.1 に戻すデスクトップ向け)の PKCE と同意の URL を作る純粋な関数(#2151 の 1)。
 * 通信も画面も持たない。受け口は [LoopbackReceiver](#2151 の 2)。
 */
object GooglePkce {
    const val AUTHORIZATION_ENDPOINT = "https://accounts.google.com/o/oauth2/v2/auth"
    const val SCOPE = "openid email profile"
    const val MIN_VERIFIER_LENGTH = 43
    const val MAX_VERIFIER_LENGTH = 128
    const val DEFAULT_VERIFIER_LENGTH = 64
    private const val STATE_BYTES = 32

    /** RFC 7636 の unreserved(`A-Z a-z 0-9 - . _ ~`) */
    private const val UNRESERVED = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

    private val base64Url = Base64.getUrlEncoder().withoutPadding()

    fun newRequest(random: SecureRandom = SecureRandom()): PkceRequest {
        val verifier = newCodeVerifier(random)
        return PkceRequest(verifier, codeChallengeS256(verifier), newState(random))
    }

    fun newCodeVerifier(random: SecureRandom = SecureRandom(), length: Int = DEFAULT_VERIFIER_LENGTH): String {
        require(length in MIN_VERIFIER_LENGTH..MAX_VERIFIER_LENGTH) { "code_verifier length must be 43..128" }
        return buildString(length) { repeat(length) { append(UNRESERVED[random.nextInt(UNRESERVED.length)]) } }
    }

    /** `BASE64URL(SHA256(ASCII(code_verifier)))`(RFC 7636 4.2) */
    fun codeChallengeS256(codeVerifier: String): String {
        require(codeVerifier.length in MIN_VERIFIER_LENGTH..MAX_VERIFIER_LENGTH && codeVerifier.all { it in UNRESERVED }) {
            "invalid code_verifier"
        }
        val digest = MessageDigest.getInstance("SHA-256").digest(codeVerifier.toByteArray(StandardCharsets.US_ASCII))
        return base64Url.encodeToString(digest)
    }

    fun newState(random: SecureRandom = SecureRandom()): String =
        base64Url.encodeToString(ByteArray(STATE_BYTES).also(random::nextBytes))

    fun redirectUri(port: Int): String {
        require(port in 1..65535) { "port out of range" }
        return "http://127.0.0.1:$port"
    }

    fun authorizationUrl(
        clientId: String,
        port: Int,
        request: PkceRequest,
        endpoint: String = AUTHORIZATION_ENDPOINT,
    ): String {
        require(clientId.isNotBlank()) { "clientId is blank" }
        val params = linkedMapOf(
            "client_id" to clientId,
            "redirect_uri" to redirectUri(port),
            "response_type" to "code",
            "scope" to SCOPE,
            "code_challenge" to request.codeChallenge,
            "code_challenge_method" to "S256",
            "state" to request.state,
        )
        return endpoint + "?" + params.entries.joinToString("&") { (k, v) -> "${encode(k)}=${encode(v)}" }
    }

    /** state の突き合わせ。長さ以外の違いで時間が変わらないよう比べる。 */
    fun statesMatch(expected: String, actual: String?): Boolean {
        if (actual == null) return false
        return MessageDigest.isEqual(
            expected.toByteArray(StandardCharsets.UTF_8),
            actual.toByteArray(StandardCharsets.UTF_8),
        )
    }

    private fun encode(s: String): String = URLEncoder.encode(s, StandardCharsets.UTF_8).replace("+", "%20")
}
