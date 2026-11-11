package jp.kz2601.metamon.auth

import android.app.Activity
import android.content.Context
import androidx.credentials.CredentialManager
import androidx.credentials.CustomCredential
import androidx.credentials.GetCredentialRequest
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import com.google.android.libraries.identity.googleid.GetSignInWithGoogleOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential
import jp.kz2601.metamon.BuildConfig
import jp.kz2601.metamon.services.AuthError

/** Google サインインの Web クライアント ID(Firebase の OAuth クライアント)。 */
object GoogleWebClientId {
    /** google-services プラグインが json から作るリソース名 */
    const val RESOURCE_NAME = "default_web_client_id"

    /** BuildConfig(gradle の `metamon.<flavor>.googleWebClientId`)を優先し、空なら json 由来のリソース。どちらも無ければ null。 */
    fun resolve(buildConfigValue: String, fromResource: () -> String?): String? =
        buildConfigValue.ifBlank { null } ?: fromResource()?.ifBlank { null }

    fun resolve(context: Context): String? = resolve(BuildConfig.GOOGLE_WEB_CLIENT_ID) {
        val id = context.resources.getIdentifier(RESOURCE_NAME, "string", context.packageName)
        if (id == 0) null else context.getString(id)
    }
}

/** Credential Manager の「Google でログイン」ボタンの流れ(アカウント選択のシートを出す)。 */
class CredentialManagerGoogleSignIn(
    private val activity: Activity,
    private val webClientId: String,
) : GoogleIdTokenSource {
    override suspend fun idToken(): String {
        val request = GetCredentialRequest.Builder()
            .addCredentialOption(GetSignInWithGoogleOption.Builder(serverClientId = webClientId).build())
            .build()
        val result = try {
            CredentialManager.create(activity).getCredential(activity, request)
        } catch (e: GetCredentialCancellationException) {
            throw AuthError.Canceled(e)
        } catch (e: GetCredentialException) {
            throw AuthError.Unavailable(e.message ?: "Google でログインできませんでした")
        }
        val credential = result.credential
        if (credential is CustomCredential && credential.type == GoogleIdTokenCredential.TYPE_GOOGLE_ID_TOKEN_CREDENTIAL) {
            return GoogleIdTokenCredential.createFrom(credential.data).idToken
        }
        throw AuthError.Unavailable("Google の認証情報を受け取れませんでした")
    }
}
