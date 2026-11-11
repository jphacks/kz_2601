package jp.kz2601.metamon

import android.content.Context
import android.content.Intent
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
import androidx.compose.runtime.CompositionLocalProvider
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
import jp.kz2601.metamon.purchase.PaywallHost
import jp.kz2601.metamon.discover.DiscoverHost
import jp.kz2601.metamon.discover.LocalPublishEnv
import jp.kz2601.metamon.onboarding.TutorialScreen
import jp.kz2601.metamon.services.consent.AiConsentStatus
import jp.kz2601.metamon.feedback.FeedbackComposerHost
import jp.kz2601.metamon.feedback.FeedbackHost
import jp.kz2601.metamon.feedback.HotReloadStatusBadge
import jp.kz2601.metamon.feedback.TalkBackAnnouncer
import jp.kz2601.metamon.feedback.UndoHeaderButtons
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.notifications.LocalNotifications
import jp.kz2601.metamon.push.PushOpen
import jp.kz2601.metamon.services.push.PushMessage
import jp.kz2601.metamon.permissions.MicrophoneAccessProvider
import jp.kz2601.metamon.permissions.PermissionOffBarHost
import jp.kz2601.metamon.auth.RootScreen
import jp.kz2601.metamon.auth.SignInScreen
import jp.kz2601.metamon.permissions.ProvideRendererPermissions
import jp.kz2601.metamon.renderer.DSLView
import jp.kz2601.metamon.renderer.components.feed.LocalFeedHost
import jp.kz2601.metamon.renderer.theme.MetamonRenderTheme
import jp.kz2601.metamon.renderer.theme.ReadableWidth
import jp.kz2601.metamon.retention.RetentionPromptHost
import jp.kz2601.metamon.retention.retentionPending
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.NavigationSnapshot
import jp.kz2601.metamon.runtime.TranscriberDictationHost
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.voice.AndroidSpeechEngine
import jp.kz2601.metamon.voice.AppConfigVersions
import jp.kz2601.metamon.voice.ClarifySettings
import jp.kz2601.metamon.voice.Greeting
import jp.kz2601.metamon.voice.VoiceRoute
import jp.kz2601.metamon.voice.VoiceFab
import jp.kz2601.metamon.voice.VoiceUiState
import jp.kz2601.metamon.voice.VoiceViewModel
import jp.kz2601.metamon.voice.stage.BuildStageHost
import jp.kz2601.metamon.voice.stage.BuildStageState
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.sharing.IssuedShares
import jp.kz2601.metamon.services.sharing.MembersModel
import jp.kz2601.metamon.services.sharing.ShareScope
import jp.kz2601.metamon.services.sharing.ShareSheetModel
import jp.kz2601.metamon.settings.SettingsSheet
import jp.kz2601.metamon.share.LocalShareSheetExtras
import jp.kz2601.metamon.share.MembersSection
import jp.kz2601.metamon.share.MembersSheet
import jp.kz2601.metamon.share.ShareLinkHost
import jp.kz2601.metamon.share.ShareLinkIntents
import jp.kz2601.metamon.share.ShareSheet
import jp.kz2601.metamon.share.ShareTarget
import jp.kz2601.metamon.switcher.AppSwitcherDialog
import jp.kz2601.metamon.update.UpdateGateHost
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
        if (savedInstanceState == null) offerPushOpen(intent) else restoreNavigation(container, savedInstanceState)
        setContent {
            MetamonRenderTheme {
                MicrophoneAccessProvider { MetamonRoot(container) }
            }
        }
    }

    // singleTask: 開いている間に届いた共有リンク・通知のタップはここに来る。onCreate の初回と同じ受け口に通し、
    // 今の画面の上で参加の確認・アプリの切り替えを出す(切り替えは変身一覧と同じ AppRuntime.switchApp)
    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        offerPushOpen(intent)
    }

    // 前面・背面の処理(送信待ち・取り直し・通知の登録など)は MetamonApplication の AppForeground が持つ(#891)

    // プロセスが落とされても、戻った時に同じアプリの同じ画面を開く(#891)。記録・フォームの下書き・音声の画面は初めの状態に戻す
    private fun restoreNavigation(container: AppContainer, savedInstanceState: Bundle?) {
        container.runtime.offerNavigationRestore(NavigationSnapshot.decode(savedInstanceState?.getString(NAVIGATION_KEY)))
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        val snapshot = (application as MetamonApplication).builtContainer?.runtime?.navigationSnapshot() ?: return
        outState.putString(NAVIGATION_KEY, snapshot.encode())
    }

    private companion object {
        const val NAVIGATION_KEY = "metamon.navigation"
    }

    /** 通知のタップ(自分で出した通知も、FCM が裏で出した通知も extra の `appId`)と、シェアリンク(#901)。 */
    private fun offerPushOpen(intent: Intent?) {
        PushOpen.offer(PushMessage.appId(intent?.getStringExtra(LocalNotifications.EXTRA_APP_ID)))
        if (!ReviewBuild.enabled) ShareLinkIntents.offer(intent) // 審査用の版はシェアリンクを開かない(#1275)
    }
}

/**
 * 未サインインならサインイン画面、サインイン済みなら本体(iOS の `AuthSession.user` で出し分けるのと同じ)。
 * 見るのは持ち主の見張り(#841)が受け入れた人: 別の人に変わった時は、前の人のデータを消し終わるまで本体を出さない。
 */
@Composable
fun MetamonRoot(container: AppContainer) {
    val user by container.accountGuard.active.collectAsStateWithLifecycle()
    // お知らせのバナーと「アップデートしてください」はサインイン前にも出す(#906)
    UpdateGateHost(container.updateGate, container.inAppUpdater) {
        when (RootScreen.of(user)) {
            RootScreen.SignIn -> SignInScreen(container.session)
            RootScreen.App -> CompositionLocalProvider(LocalFeedHost provides container.feedHost) { MetamonApp(container) }
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
    val user by container.accountGuard.active.collectAsStateWithLifecycle()
    // 後片付け(#841)のたびに作り直す: 前の人の発話・要約・元に戻すの履歴を次の人に出さない
    val wipes by container.accountGuard.generation.collectAsStateWithLifecycle()
    val buildNow = stringResource(R.string.voice_build_now_utterance)
    val historyHeader = stringResource(R.string.voice_history_header)
    val vm = viewModel(key = "voice-$wipes") {
        val clarify = ClarifySettings(context)
        VoiceViewModel(
            transcriber = container.transcriber,
            ai = container.ai,
            appId = { container.appId.current },
            initial = AppDocument(appId = container.appId.current, dsl = container.initialDsl),
            buildNowUtterance = buildNow,
            clarifyEnabled = { clarify.enabled },
            autoBuild = { clarify.autoBuild },
            // 会話の履歴はアプリごとに端末へ(#893)。サインアウト・削除で消すので container が持つ
            chatHistory = container.chatHistory,
            historyHeader = historyHeader,
            // 発話・AI の返事を含むので debug だけ(release の logcat に個人の情報を出さない)
            log = { if (BuildConfig.DEBUG) Log.i(AppContainer.TAG, it) },
            // モックはサーバーの版の履歴が無いので端末の履歴だけで戻す
            versions = if (container.knobs.mock) null else AppConfigVersions(container.appConfig),
            // 未同意なら送らずに同意の画面を出す。同意して閉じたら、止めた 1 件を続けて送る(#888)
            requireConsent = container.consentPrompt::require,
            // 挨拶・AI の質問・完成の要約の読み上げ(#892・設定で ON の時だけ)
            speech = AndroidSpeechEngine.reader(context),
            // 「今日の残り」(`GET /v1/me/usage`・iOS と同じ口)。モックは出さない
            usage = if (container.knobs.mock) null else container.appConfig as? PlanUsageSource,
        )
    }
    val state by vm.state.collectAsStateWithLifecycle()
    val runtime = container.runtime
    // 保存期間の確認が待っているか(完成の後のお知らせの順番・#1271)
    val retentionWaiting = retentionPending(runtime)
    val runtimeState by runtime.state.collectAsStateWithLifecycle()
    val hotReloadStatus by runtime.hotReloadStatus.collectAsStateWithLifecycle()

    LaunchedEffect(runtime) { runtime.bootstrap() }
    // AI の質問・返事・完成の要約・状態カード・トーストを出たのと同時に TalkBack に読ませる(H8)
    TalkBackAnnouncer(vm.state, runtime.state)
    // 初回起動・説明の版が上がった時に 1 回だけ同意の画面を出す(iOS の ConsentHost・#888)
    LaunchedEffect(Unit) {
        delay(CONSENT_PROMPT_DELAY_MILLIS)
        container.consentPrompt.showOnLaunchIfNeeded()
    }
    // 同意の画面が閉じたら(要らない時は起動して少し待って)初回のチュートリアルを出す(iOS の TutorialHost・#900)
    val consentVisible by container.consentPrompt.visible.collectAsStateWithLifecycle()
    LaunchedEffect(consentVisible) {
        if (consentVisible) return@LaunchedEffect
        delay(TUTORIAL_PROMPT_DELAY_MILLIS)
        container.tutorialPrompt.presentIfNeeded(
            consentPrompted = container.consent.state.value.status != AiConsentStatus.NOT_PROMPTED,
            consentShowing = container.consentPrompt.visible.value,
        )
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
    // マイク FAB の音声入力が出ている間は transcribeField の録音を捨てる(#864)
    LaunchedEffect(state.voiceOpen) { (runtime.dictation as? TranscriberDictationHost)?.setVoiceInputActive(state.voiceOpen) }
    // ランタイムが開いたアプリ(起動時の開き直し・切り替え・ホットリロード)を音声の画面にも映す
    LaunchedEffect(runtimeState.document) { vm.adopt(runtimeState.document) }

    // サインイン(uid の変化)で端末の登録を確かめる(#887)
    LaunchedEffect(user?.uid) { container.push?.refresh() }
    // 通知のタップで該当アプリを開く
    val pushOpen by PushOpen.pending.collectAsStateWithLifecycle()
    LaunchedEffect(pushOpen) {
        val appId = PushOpen.consume() ?: return@LaunchedEffect
        container.push?.open(appId) { openFromNotification(runtime, it) }
    }

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
    // 共有シート(#901)。発行したリンクは人が変わる(wipes)まで覚える。モックは mockSharing の時だけ
    var shareTarget by remember { mutableStateOf<ShareTarget?>(null) }
    val issuedShares = remember(wipes) { IssuedShares() }
    // 審査用の版(#1275)は共有の入口を出さない
    val canShare = ReviewBuild.canShare(ReviewBuild.enabled, container.knobs.mock, container.knobs.mockSharing)
    // 「探す」(#902)。サーバーにつながる構成だけ入口を出す(iOS の canShare)
    var discoverOpen by rememberSaveable { mutableStateOf(false) }
    // 参加している人(#903)。入口は共有シートの下と設定
    var membersTarget by remember { mutableStateOf<ShareTarget?>(null) }
    val membersClient = container.members.takeIf { canShare }
    fun loadDsl(appId: String): suspend () -> AppDSL = {
        val now = runtime.state.value
        if (now.document.appId == appId) now.dsl else container.appConfig.fetchDocument(appId).document.dsl
    }
    fun membersModel(target: ShareTarget) =
        MembersModel(target.appId, membersClient, isShared = { ShareScope.of(loadDsl(target.appId)()) == ShareScope.MEMBERS }, container.membersStore)
    // 抜けた後: シートを閉じて知らせるだけ。アプリと自分のコピーは一覧に残す(iOS と同じ・#1298)
    val onLeftShare = { target: ShareTarget ->
        shareTarget = null
        membersTarget = null
        issuedShares[target.appId] = null
        val name: Any = target.name.ifEmpty { null } ?: UiText.Res(R.string.share_untitled)
        runtime.showToast(UiText.Res(R.string.members_left_toast, name))
    }

    val openSwitcher = {
        switcherOpen = true
        scope.launch { runtime.appsList() }
        Unit
    }

    Scaffold(
        bottomBar = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // アプリの切り替えの入口(最小・iOS は FAB の長押し。見た目は子 Issue)
                TextButton(onClick = openSwitcher) { Text(stringResource(R.string.switcher_open)) }
                TextButton(onClick = { settingsOpen = true }, modifier = Modifier.testTag("settings.open")) {
                    Text(stringResource(R.string.settings_title))
                }
                if (container.discover != null) {
                    TextButton(onClick = { discoverOpen = true }, enabled = !state.isGenerating, modifier = Modifier.testTag("discover.open")) {
                        Text(stringResource(R.string.discover_entry_short))
                    }
                }
                if (!container.environment.isProduction || container.knobs.mock) {
                    Text(
                        "${container.environmentLabel} / auth=${container.auth.kind} / uid=${user?.uid ?: "-"}",
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(8.dp),
                    )
                    // ホットリロードが SSE か・ポーリングに落ちているか(#1108)
                    HotReloadStatusBadge(hotReloadStatus)
                }
            }
        },
    ) { padding ->
        Box(Modifier.fillMaxSize().padding(padding)) {
            // 生成中の組み立て台・変わった部品の印・完成の後のお知らせ(#894)
            BuildStageHost(
                state,
                onCancel = vm::cancelGenerate,
                onOpenVoice = vm::openVoice,
                editHints = container.tutorial.editHints,
                retentionPending = retentionWaiting,
            ) {
                if (state.isInitial) {
                    VoiceRoute(vm, onMicLongPress = openSwitcher)
                } else {
                    // 横向き・タブレット・折りたたみでは幅を絞って中央に寄せる(iOS の readableWidth・#842)。
                    ReadableWidth {
                        GeneratedApp(state, runtimeState.dsl, runtime, appId = runtimeState.document.appId)
                    }
                    // マイク FAB(#892): タップ = このアプリを変える・長押し = 変身一覧・ドラッグで動かす / 端に隠す
                    VoiceFab(busy = state.isGenerating, onTap = vm::openVoice, onLongPress = openSwitcher)
                }
            }
            // 要約と元に戻す・状態カード・トースト・削除の取り消し・確認ダイアログ(#895)
            FeedbackHost(vm, state, runtime, runtimeState)
        }
    }

    if (settingsOpen) {
        SettingsSheet(
            container,
            busy = state.isGenerating,
            onDismiss = { settingsOpen = false },
            // 一度も作っていないアプリ(version 1)はシェアできない(backend が 400)
            onMembers = if (membersClient != null && ShareScope.of(runtimeState.dsl) == ShareScope.MEMBERS) {
                {
                    settingsOpen = false
                    membersTarget = ShareTarget(runtimeState.document.appId, runtimeState.dsl.app.name)
                }
            } else {
                null
            },
            onShare = if (canShare && runtimeState.document.version > 1) {
                {
                    settingsOpen = false
                    shareTarget = ShareTarget(runtimeState.document.appId, runtimeState.dsl.app.name)
                }
            } else {
                null
            },
            remainingToday = state.remainingToday,
            isPro = state.isPro,
            myUsage = state.myUsage,
            onRefreshUsage = { vm.refreshUsage() },
            onStopRetention = vm::generateNow,
            onShowTutorial = {
                settingsOpen = false
                container.tutorialPrompt.replay()
            },
            canUndo = state.canUndo && !state.undoing && !state.isGenerating,
            onUndo = {
                settingsOpen = false
                vm.undo(state.document.version)
            },
            onPickTip = { text ->
                settingsOpen = false
                vm.putDraft(text)
            },
            onReset = {
                settingsOpen = false
                runtime.startNewApp()
                vm.noteHomeEntry(Greeting.HomeEntry.HOME)
            },
        )
    }
    // 設定のシートより後に置く(設定から出した時に上に重ねる)
    if (consentVisible) {
        val consentState by container.consent.state.collectAsStateWithLifecycle()
        AiConsentDialog(
            needsReconsent = consentState.needsReconsent,
            onAgree = container.consentPrompt::agree,
            onLater = container.consentPrompt::later,
        )
    }
    // 初回のチュートリアル(#900)。同意の画面とは重ねない(閉じてから出す)
    val tutorialVisible by container.tutorialPrompt.visible.collectAsStateWithLifecycle()
    if (tutorialVisible && !consentVisible) {
        TutorialScreen(onFinish = container.tutorialPrompt::finish)
    }

    // 記録の保存期間の確認(#482)。完成の後のお知らせの順番(要約の後・#1271)で、生成中・音声のシートの間は待つ
    RetentionPromptHost(
        runtime,
        busy = state.isGenerating || state.voiceOpen,
        turn = BuildStageState.isRetentionTurn(state, retentionWaiting),
        onStop = vm::generateNow,
    )
    // フィードバックの画面(#319)と端末を振って開く仕組み。設定から開いた時に上に重ねる
    FeedbackComposerHost(container)
    // Tailor It Pro の課金画面(AI ボタン・設定から)。サインインのたびにサーバーの Pro を読み直す
    PaywallHost(container.pro, user?.uid)

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
                vm.noteHomeEntry(Greeting.HomeEntry.NEW_APP) // 挨拶を「新しいアプリですね」に
            },
            onDismiss = { switcherOpen = false },
            onShare = if (canShare) { app ->
                switcherOpen = false
                shareTarget = ShareTarget(app.id, app.name)
            } else {
                null
            },
        )
    }

    // アプリの共有シート(#901)。入口は変身一覧の各行と設定
    shareTarget?.let { target ->
        val model = remember(target, issuedShares) {
            ShareSheetModel(target.appId, container.appConfig.takeIf { canShare }, issuedShares, loadDsl(target.appId))
        }
        val members = remember(target) { membersModel(target) }
        CompositionLocalProvider(
            // 下半分の「みんなに公開」(#902)が使う
            LocalPublishEnv provides container.publishEnv.takeIf { canShare },
            LocalShareSheetExtras provides { MembersSection(members, onRevokeLink = { model.revoke() }, onLeft = { onLeftShare(target) }, leaveScope = container.accountScope) },
        ) {
            ShareSheet(target, model, onDismiss = { shareTarget = null })
        }
    }
    membersTarget?.let { target ->
        val members = remember(target) { membersModel(target) }
        MembersSheet(members, onLeft = { onLeftShare(target) }, onDismiss = { membersTarget = null }, leaveScope = container.accountScope)
    }
    // シェアリンクから開く・参加する(#901)
    ShareLinkHost(container, canShare)
    // みんなが作ったアプリを探す・入れる(#902)
    DiscoverHost(container, discoverOpen, onDismiss = { discoverOpen = false })

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

/**
 * 通知を押した時に appId のアプリを開く(#887)。今開いているアプリなら何もしない。
 * 別のアプリは [AppRuntime.switchApp](#1069)で開く。読めなければ今のアプリのまま(切り替え側がトーストを出す)。
 */
private suspend fun openFromNotification(runtime: AppRuntime, appId: String) {
    if (runtime.document.appId == appId) return
    if (!runtime.switchApp(appId)) Log.i(AppContainer.TAG, "push: could not open the app from the notification")
}

/** 生成の途中経過を 1 行にする。今の画面の途中経過は [BuildStageHost] が出す(文言の置き場として残す)。 */
internal fun progressText(p: GenerateProgress): UiText = when (p) {
    is GenerateProgress.App ->
        if (p.tabs.isEmpty()) UiText.Res(R.string.voice_progress_app, p.name)
        else UiText.Res(R.string.voice_progress_app_tabs, p.name, p.tabs.joinToString("・"))
    is GenerateProgress.Summary -> UiText.Raw(p.text)
    is GenerateProgress.Component -> UiText.Res(R.string.voice_progress_component, p.component.index + 1, p.component.label ?: p.component.type)
    is GenerateProgress.Restart -> UiText.Res(R.string.voice_progress_restart)
}

private const val AUTO_NAV_START_MILLIS = 3_000L

/** 起動の画面が落ち着いてから同意の画面を出す(iOS の ConsentHost と同じ 0.5 秒) */
private const val CONSENT_PROMPT_DELAY_MILLIS = 500L

/** 同意の画面が閉じきってから(要らない時は起動の画面が落ち着いてから)チュートリアルを出す(iOS の TutorialHost の 0.7〜0.8 秒) */
private const val TUTORIAL_PROMPT_DELAY_MILLIS = 800L
