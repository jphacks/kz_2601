package jp.kz2601.metamon.auth

import android.app.Activity
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.launch

/** 起動時にサインインしていなければ出す画面(iOS の `SignInView`)。Google・Apple・匿名。 */
@Composable
fun SignInScreen(session: AuthSession) {
    val context = LocalContext.current
    val activity = context as? Activity
    val scope = rememberCoroutineScope()
    val busy by session.isBusy.collectAsStateWithLifecycle()
    val message by session.errorMessage.collectAsStateWithLifecycle()
    val webClientId = remember(context) { GoogleWebClientId.resolve(context) }

    Box(Modifier.fillMaxSize().padding(24.dp)) {
        Column(
            modifier = Modifier.fillMaxSize(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Spacer(Modifier.weight(1f))
            Text("Tailor It", fontSize = 40.sp, fontWeight = FontWeight.Bold)
            Text(
                "話しかけるだけで、アプリが生まれ変わる",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.weight(1f))

            // Web クライアント ID が無いビルド(json 無し・未設定)では Google のボタンを出さない
            if (activity != null && webClientId != null) {
                Button(
                    onClick = { scope.launch { session.signInWithGoogle(CredentialManagerGoogleSignIn(activity, webClientId)) } },
                    enabled = !busy,
                    modifier = Modifier.fillMaxWidth().height(52.dp),
                ) { Text("Google でログイン") }
            }
            if (activity != null) {
                OutlinedButton(
                    onClick = { scope.launch { session.signInWithApple(activity) } },
                    enabled = !busy,
                    modifier = Modifier.fillMaxWidth().height(52.dp),
                ) { Text("Apple でログイン") }
            }
            TextButton(onClick = { scope.launch { session.signInAnonymously() } }, enabled = !busy) {
                Text("ログインせずに使う(匿名)", color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            message?.let {
                Text(it, color = MaterialTheme.colorScheme.error, textAlign = TextAlign.Center)
            }
        }
        if (busy) CircularProgressIndicator(Modifier.align(Alignment.Center))
    }
}
