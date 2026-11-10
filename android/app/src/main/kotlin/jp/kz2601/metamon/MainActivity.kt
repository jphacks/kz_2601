package jp.kz2601.metamon

import android.os.Bundle
import android.util.Log
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.renderer.theme.ReadableWidth

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as MetamonApplication).container
        setContent {
            MetamonRenderTheme {
                MetamonApp(container)
            }
        }
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

    LaunchedEffect(container.auth) {
        if (container.auth.currentUser.value == null) {
            runCatching { container.auth.signInAnonymously() }
                .onFailure { Log.w("Metamon", "匿名サインインに失敗", it) }
        }
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
        // 横向き・タブレット・折りたたみでは幅を絞って中央に寄せる(iOS の readableWidth・#842)。
        ReadableWidth(Modifier.padding(padding)) {
            Column(Modifier.fillMaxSize()) {
                DSLView(container.initialDsl, runtime)
            }
        }
    }
}
