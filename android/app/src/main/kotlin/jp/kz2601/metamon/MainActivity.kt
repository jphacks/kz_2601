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
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.http.FailureKind
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.io.File

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as MetamonApplication).container(DebugLaunch.knobs(intent))
        setContent {
            MetamonRenderTheme {
                MetamonApp(container)
            }
        }
    }
}

/**
 * 骨格の画面: DSL を描き、下に接続先と認証の状態を出す。
 * 文字起こしがある構成(今はモックだけ)では、音声 → 生成(途中経過付き)→ 描き直し まで通る。
 */
@Composable
fun MetamonApp(container: AppContainer) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val user by container.auth.currentUser.collectAsStateWithLifecycle()
    var dsl by remember { mutableStateOf(container.initialDsl) }
    var version by remember { mutableIntStateOf(0) }
    var status by remember { mutableStateOf<String?>(null) }

    suspend fun generate(transcript: String) {
        status = "生成中: $transcript"
        runCatching {
            container.ai.morphStreaming(transcript, appId = LOCAL_APP_ID, currentVersion = version) { status = progressText(it) }
        }.onSuccess {
            dsl = it.document.dsl
            version = it.document.version
            status = it.summary
        }.onFailure {
            Log.w(AppContainer.TAG, "生成に失敗", it)
            status = "生成に失敗: ${FailureKind.classify(it)}"
        }
    }

    val runtime = remember {
        AppRuntime {
            val transcriber = container.transcriber
            if (transcriber == null) {
                Toast.makeText(context, "音声入力は準備中です(#359)", Toast.LENGTH_SHORT).show()
            } else scope.launch {
                transcriber.startRecording()
                status = "聞いています…"
                generate(transcriber.stopAndTranscribe())
            }
        }
    }

    LaunchedEffect(container.auth) {
        if (container.auth.currentUser.value == null) {
            runCatching { container.auth.signInAnonymously() }
                .onFailure { Log.w(AppContainer.TAG, "匿名サインインに失敗", it) }
        }
    }

    // ヘッドレス E2E 用のつまみ(debug だけ値が入る)
    LaunchedEffect(Unit) {
        val knobs = container.knobs
        knobs.autoTranscript?.let { generate(it) }
        if (knobs.autoNav.isEmpty()) return@LaunchedEffect
        delay(AUTO_NAV_START_MILLIS)
        knobs.autoNav.forEachIndexed { i, step ->
            // 遷移(#357)が入るまでは initialScreen の差し替えで画面を出す。出し方(sheet など)はまだ見ない
            val found = dsl.screen(step.screen) != null
            if (found) dsl = dsl.copy(initialScreen = step.screen)
            val line = "auto nav ${i + 1}/${knobs.autoNav.size}: → $step ${if (found) "ok" else "not found"}"
            Log.i(AppContainer.TAG, line)
            // logcat はまとめて流れることがあるので、スクショ側が同期できるよう cache/autonav-step にも書く
            runCatching { File(context.cacheDir, "autonav-step").writeText(line) }
            delay((knobs.autoNavIntervalSeconds * 1000).toLong())
        }
    }

    Scaffold(
        bottomBar = {
            if (!container.environment.isProduction || container.knobs.mock) {
                Column(Modifier.padding(8.dp)) {
                    status?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                    Text(
                        "${container.environmentLabel} / auth=${container.auth.kind} / uid=${user?.uid ?: "-"}",
                        style = MaterialTheme.typography.labelSmall,
                    )
                }
            }
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            DSLView(dsl, runtime)
        }
    }
}

/** 生成の途中経過を 1 行にする(仮置き・最後の結果で置き直す)。 */
internal fun progressText(p: GenerateProgress): String = when (p) {
    is GenerateProgress.App -> "「${p.name}」を作っています" + if (p.tabs.isEmpty()) "" else "(${p.tabs.joinToString("・")})"
    is GenerateProgress.Summary -> p.text
    is GenerateProgress.Component -> "部品 ${p.component.index + 1}: ${p.component.label ?: p.component.type}"
    is GenerateProgress.Restart -> "やり直しています"
}

private const val LOCAL_APP_ID = "local"
private const val AUTO_NAV_START_MILLIS = 3_000L
