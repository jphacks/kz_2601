package jp.kz2601.metamon

import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.auth.RootScreen
import jp.kz2601.metamon.auth.SignInScreen
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as MetamonApplication).container
        setContent {
            MetamonRenderTheme {
                MetamonRoot(container)
            }
        }
    }
}

/** 未サインインならサインイン画面、サインイン済みなら本体(iOS の `AuthSession.user` で出し分けるのと同じ)。 */
@Composable
fun MetamonRoot(container: AppContainer) {
    val user by container.session.user.collectAsStateWithLifecycle()
    when (RootScreen.of(user)) {
        RootScreen.SignIn -> SignInScreen(container.session)
        RootScreen.App -> MetamonApp(container)
    }
}

/** 骨格の画面: 初期の DSL(音声の入口)を描き、下に接続先と認証の状態を出す。 */
@Composable
fun MetamonApp(container: AppContainer) {
    val context = LocalContext.current
    val user by container.auth.currentUser.collectAsStateWithLifecycle()
    val runtime = remember {
        AppRuntime { Toast.makeText(context, "音声入力は準備中です(#359)", Toast.LENGTH_SHORT).show() }
    }

    Scaffold(
        bottomBar = {
            if (!container.environment.isProduction) {
                Text(
                    "${container.environment.name} / auth=${container.auth.kind} / uid=${user?.uid ?: "-"}",
                    style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.padding(8.dp),
                )
            }
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            DSLView(container.initialDsl, runtime)
        }
    }
}
