package jp.kz2601.metamon

import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.voice.GeneratingView
import jp.kz2601.metamon.voice.MicButton
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceRoute
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.VoiceViewModel
import kotlinx.coroutines.delay
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
 * 初期状態は音声の画面(iOS の RootView → VoiceInputView)。生成したら DSL を描き、マイク FAB から音声のシートで変える。
 * 描画は今の [DSLView] のまま(部品のレジストリ・グリッドは #986 側)。
 * モックの構成(`METAMON_MOCK=1`)でも同じ画面を通る(文字起こし・生成はモック)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MetamonApp(container: AppContainer) {
    val context = LocalContext.current
    val user by container.auth.currentUser.collectAsStateWithLifecycle()
    val buildNow = stringResource(R.string.voice_build_now_utterance)
    val vm = viewModel {
        VoiceViewModel(
            transcriber = container.transcriber,
            ai = container.ai,
            appId = { container.appId.current },
            initial = AppDocument(appId = container.appId.current, dsl = container.initialDsl),
            buildNowUtterance = buildNow,
            log = { Log.i(AppContainer.TAG, it) },
        )
    }
    val state by vm.state.collectAsStateWithLifecycle()
    val runtime = remember(vm) { AppRuntime { vm.openVoice() } }

    LaunchedEffect(container.auth) {
        if (container.auth.currentUser.value == null) {
            runCatching { container.auth.signInAnonymously() }
                .onFailure { Log.w(AppContainer.TAG, "匿名サインインに失敗", it) }
        }
    }

    // ヘッドレス E2E 用のつまみ(debug だけ値が入る)
    LaunchedEffect(vm) {
        val knobs = container.knobs
        knobs.autoTranscript?.let { vm.generateNow(it) }
        if (knobs.autoNav.isEmpty()) return@LaunchedEffect
        delay(AUTO_NAV_START_MILLIS)
        knobs.autoNav.forEachIndexed { i, step ->
            // 遷移(#357)が入るまでは initialScreen の差し替えで画面を出す。出し方(sheet など)はまだ見ない
            val found = vm.showScreen(step.screen)
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
                Text(
                    "${container.environmentLabel} / auth=${container.auth.kind} / uid=${user?.uid ?: "-"}",
                    style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.padding(8.dp),
                )
            }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (state.isInitial) {
                VoiceRoute(vm)
            } else {
                GeneratedApp(state, runtime, onDismissSummary = vm::dismissSummary)
                MicButton(
                    state.copy(phase = VoicePhase.IDLE),
                    size = 64.dp,
                    onClick = vm::openVoice,
                    description = stringResource(R.string.voice_change_this_app),
                    modifier = Modifier.align(Alignment.BottomEnd).padding(20.dp),
                )
            }
            if (state.isGenerating) GeneratingView(state, onCancel = vm::cancelGenerate)
        }
    }

    if (!state.isInitial && state.voiceOpen) {
        ModalBottomSheet(onDismissRequest = vm::closeVoice, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
            VoiceRoute(vm, Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun GeneratedApp(state: VoiceUiState, runtime: AppRuntime, onDismissSummary: () -> Unit) {
    Column(Modifier.fillMaxSize()) {
        state.summary?.let { summary ->
            OutlinedCard(Modifier.fillMaxWidth().padding(12.dp).clickable(onClick = onDismissSummary)) {
                Text(summary, Modifier.padding(12.dp), style = MaterialTheme.typography.bodyMedium)
            }
        }
        DSLView(state.document.dsl, runtime)
    }
}

/** 生成の途中経過を 1 行にする。今の画面の途中経過は [GeneratingView] が出す(文言の置き場として残す)。 */
internal fun progressText(p: GenerateProgress): String = when (p) {
    is GenerateProgress.App -> "「${p.name}」を作っています" + if (p.tabs.isEmpty()) "" else "(${p.tabs.joinToString("・")})"
    is GenerateProgress.Summary -> p.text
    is GenerateProgress.Component -> "部品 ${p.component.index + 1}: ${p.component.label ?: p.component.type}"
    is GenerateProgress.Restart -> "やり直しています"
}

private const val AUTO_NAV_START_MILLIS = 3_000L
