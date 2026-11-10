package jp.kz2601.metamon

import android.content.Context
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.foundation.layout.Row
import androidx.compose.material3.TextButton
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.consent.AiConsentDialog
import jp.kz2601.metamon.feedback.FeedbackHost
import jp.kz2601.metamon.feedback.UndoHeaderButtons
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.permissions.PermissionOffBarHost
import jp.kz2601.metamon.auth.RootScreen
import jp.kz2601.metamon.auth.SignInScreen
import jp.kz2601.metamon.permissions.ProvideRendererPermissions
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.renderer.theme.ReadableWidth
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.voice.AppConfigVersions
import jp.kz2601.metamon.voice.GeneratingView
import jp.kz2601.metamon.voice.MicButton
import jp.kz2601.metamon.voice.VoicePhase
import jp.kz2601.metamon.voice.VoiceRoute
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.VoiceViewModel
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.settings.SettingsSheet
import jp.kz2601.metamon.switcher.AppSwitcherDialog
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.io.File

class MainActivity : ComponentActivity() {
    // Android 12 / 12L はアプリ内で選んだ言語をここで当てる(13+ はシステム・#843)
    override fun attachBaseContext(newBase: Context) {
        super.attachBaseContext(AppLocales.wrap(newBase))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as MetamonApplication).container(DebugLaunch.knobs(intent))
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
            // 発話・AI の返事を含むので debug だけ(release の logcat に個人の情報を出さない)
            log = { if (BuildConfig.DEBUG) Log.i(AppContainer.TAG, it) },
            // モックはサーバーの版の履歴が無いので端末の履歴だけで戻す
            versions = if (container.knobs.mock) null else AppConfigVersions(container.appConfig),
            // 未同意なら送らずに同意の画面を出す。同意して閉じたら、止めた 1 件を続けて送る(#888)
            requireConsent = container.consentPrompt::require,
        )
    }
    val state by vm.state.collectAsStateWithLifecycle()
    val runtime = container.runtime
    val runtimeState by runtime.state.collectAsStateWithLifecycle()

    LaunchedEffect(runtime) { runtime.bootstrap() }
    // 初回起動・説明の版が上がった時に 1 回だけ同意の画面を出す(iOS の ConsentHost・#888)
    LaunchedEffect(Unit) {
        delay(CONSENT_PROMPT_DELAY_MILLIS)
        container.consentPrompt.showOnLaunchIfNeeded()
    }
    // 部品のマイク(onVoiceInput)は音声のシートを開く
    LaunchedEffect(runtimeState.showVoiceSheet) {
        if (runtimeState.showVoiceSheet) {
            vm.openVoice()
            runtime.dismissVoiceSheet()
        }
    }
    // 生成の結果をランタイムに渡す(別の appId なら新しいアプリとして開いて保存し、記録を読む)
    LaunchedEffect(state.document) { runtime.adoptGenerated(state.document) }
    // ランタイムが開いたアプリ(起動時の開き直し・切り替え・ホットリロード)を音声の画面にも映す
    LaunchedEffect(runtimeState.document) { vm.adopt(runtimeState.document) }

    // ヘッドレス E2E 用のつまみ(debug だけ値が入る)。回転などで作り直しても 1 回だけ
    var knobsRan by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(vm) {
        if (knobsRan) return@LaunchedEffect
        knobsRan = true
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

    var switcherOpen by remember { mutableStateOf(false) }
    var settingsOpen by rememberSaveable { mutableStateOf(false) }
    val apps by runtime.apps.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    Scaffold(
        bottomBar = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // アプリの切り替えの入口(最小・iOS は FAB の長押し。見た目は子 Issue)
                TextButton(onClick = {
                    switcherOpen = true
                    scope.launch { runtime.appsList() }
                }) { Text(stringResource(R.string.switcher_open)) }
                TextButton(onClick = { settingsOpen = true }, modifier = Modifier.testTag("settings.open")) {
                    Text(stringResource(R.string.settings_title))
                }
                if (!container.environment.isProduction || container.knobs.mock) {
                    Text(
                        "${container.environmentLabel} / auth=${container.auth.kind} / uid=${user?.uid ?: "-"}",
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(8.dp),
                    )
                }
            }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (state.isInitial) {
                VoiceRoute(vm)
            } else {
                // 横向き・タブレット・折りたたみでは幅を絞って中央に寄せる(iOS の readableWidth・#842)。
                ReadableWidth {
                    GeneratedApp(state, runtimeState.dsl, runtime, appId = runtimeState.document.appId)
                }
                MicButton(
                    state.copy(phase = VoicePhase.IDLE),
                    size = 64.dp,
                    onClick = vm::openVoice,
                    description = stringResource(R.string.voice_change_this_app),
                    modifier = Modifier.align(Alignment.BottomEnd).padding(20.dp),
                )
            }
            if (state.isGenerating) GeneratingView(state, onCancel = vm::cancelGenerate)
            // 要約と元に戻す・状態カード・トースト・削除の取り消し・確認ダイアログ(#895)
            FeedbackHost(vm, state, runtime, runtimeState)
        }
    }

    if (settingsOpen) SettingsSheet(container, onDismiss = { settingsOpen = false })
    // 設定のシートより後に置く(設定から出した時に上に重ねる)
    val consentVisible by container.consentPrompt.visible.collectAsStateWithLifecycle()
    if (consentVisible) {
        val consentState by container.consent.state.collectAsStateWithLifecycle()
        AiConsentDialog(
            needsReconsent = consentState.needsReconsent,
            onAgree = container.consentPrompt::agree,
            onLater = container.consentPrompt::later,
        )
    }

    if (switcherOpen) {
        AppSwitcherDialog(
            apps = AppSummary.visible(apps, current = runtimeState.document.appId),
            currentAppId = runtimeState.document.appId,
            onSelect = { id ->
                switcherOpen = false
                scope.launch { runtime.switchApp(id) }
            },
            onNewApp = {
                switcherOpen = false
                runtime.startNewApp()
            },
            onDismiss = { switcherOpen = false },
        )
    }

    if (!state.isInitial && state.voiceOpen) {
        ModalBottomSheet(onDismissRequest = vm::closeVoice, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
            UndoHeaderButtons(state, vm)
            VoiceRoute(vm, Modifier.fillMaxWidth())
        }
    }
}

@Composable
private fun GeneratedApp(state: VoiceUiState, dsl: AppDSL, runtime: AppRuntime, appId: String) {
    Column(Modifier.fillMaxSize()) {
        // 部品が使う権限が断られていたら上に「オフ」のバー(#890・iOS の NotificationsOffBar)。要約は FeedbackHost が出す(#895)
        PermissionOffBarHost(dsl, appId, busy = state.isGenerating, summaryShowing = state.summary != null)
        // カメラなど部品が使う権限は使う直前にアプリの窓口で聞く(#862)
        ProvideRendererPermissions { DSLView(dsl, runtime) }
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

/** 起動の画面が落ち着いてから同意の画面を出す(iOS の ConsentHost と同じ 0.5 秒) */
private const val CONSENT_PROMPT_DELAY_MILLIS = 500L
