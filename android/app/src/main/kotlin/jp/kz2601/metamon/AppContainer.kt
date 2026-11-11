package jp.kz2601.metamon

import android.content.Context
import android.util.Log
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.consent.record
import jp.kz2601.metamon.discover.DiscoverServices
import jp.kz2601.metamon.discover.PublishEnv
import jp.kz2601.metamon.runtime.AiTextAccess
import jp.kz2601.metamon.runtime.AndroidRuntimeHost
import jp.kz2601.metamon.runtime.AppAiHost
import jp.kz2601.metamon.runtime.AppDirectory
import jp.kz2601.metamon.runtime.AppFeedHost
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.KeyValueRetentionConsent
import jp.kz2601.metamon.runtime.RecordRetentionRunner
import jp.kz2601.metamon.runtime.RetentionConsentStore
import jp.kz2601.metamon.runtime.RuntimeHost
import jp.kz2601.metamon.runtime.SocialServices
import jp.kz2601.metamon.runtime.TranscriberDictationHost
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.fallback
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.notifications.LocalNotifications
import jp.kz2601.metamon.services.sharing.AppMembersStore
import jp.kz2601.metamon.services.sharing.HttpMembersClient
import jp.kz2601.metamon.services.sharing.MembersClient
import jp.kz2601.metamon.push.FcmTokens
import jp.kz2601.metamon.push.PushHost
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppEnvironment
import jp.kz2601.metamon.account.AccountGuard
import jp.kz2601.metamon.account.ActiveOwner
import jp.kz2601.metamon.account.LocalDataWipe
import jp.kz2601.metamon.account.MediaTempFiles
import jp.kz2601.metamon.account.OwnerGatedTokens
import jp.kz2601.metamon.auth.AuthSession
import jp.kz2601.metamon.consent.ConsentPrompt
import jp.kz2601.metamon.onboarding.FirstRunTutorialStore
import jp.kz2601.metamon.onboarding.TutorialPrompt
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.share.ShareLinkIntents
import jp.kz2601.metamon.services.account.AccountClient
import jp.kz2601.metamon.services.account.HttpAccountClient
import jp.kz2601.metamon.services.account.MockAccountClient
import jp.kz2601.metamon.services.local.AccountOwnerStore
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.CrashReporter
import jp.kz2601.metamon.services.NoopCrashReporter
import jp.kz2601.metamon.services.SpeechTranscriber
import jp.kz2601.metamon.services.consent.AiButtonMetricsRecorder
import jp.kz2601.metamon.feedback.FeedbackComposerLauncher
import jp.kz2601.metamon.services.feedback.FeedbackClient
import jp.kz2601.metamon.services.feedback.HttpFeedbackClient
import jp.kz2601.metamon.services.feedback.MockFeedbackClient
import jp.kz2601.metamon.services.consent.AiButtonMetricsSender
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.ConsentGatedAiClient
import jp.kz2601.metamon.services.consent.CrashReportingPreference
import jp.kz2601.metamon.services.consent.HttpAiButtonMetricsSender
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.voice.ChatHistoryStore
import jp.kz2601.metamon.voice.ClarifySettings
import jp.kz2601.metamon.voice.FabPositionStore
import jp.kz2601.metamon.voice.LaunchVisits
import jp.kz2601.metamon.voice.SpeechSettings
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.components.RemoteImages
import jp.kz2601.metamon.renderer.components.audio.LocalAudioStore
import jp.kz2601.metamon.renderer.components.dashcam.DashcamStore
import jp.kz2601.metamon.renderer.components.media.VideoCodec
import jp.kz2601.metamon.services.http.AuthTokenProvider
import jp.kz2601.metamon.services.http.ClientInfo
import jp.kz2601.metamon.services.http.DeviceContext
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.live.FirebaseAppCheckTokenProvider
import jp.kz2601.metamon.services.live.FirebaseAuthService
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import jp.kz2601.metamon.services.live.FirebaseCrashReporter
import jp.kz2601.metamon.services.live.HttpAIClient
import jp.kz2601.metamon.services.live.HttpAppConfigClient
import jp.kz2601.metamon.services.live.HttpDiscoverClient
import jp.kz2601.metamon.services.live.HttpFeedClient
import jp.kz2601.metamon.services.live.HttpFollowsClient
import jp.kz2601.metamon.services.live.HttpMatchingClient
import jp.kz2601.metamon.services.live.HttpModerationClient
import jp.kz2601.metamon.services.live.HttpPushDeviceClient
import jp.kz2601.metamon.services.local.AndroidConnectivity
import jp.kz2601.metamon.services.local.AppListCache
import jp.kz2601.metamon.services.local.CurrentAppStore
import jp.kz2601.metamon.services.local.DocumentCache
import jp.kz2601.metamon.services.feeds.FeedCache
import jp.kz2601.metamon.renderer.components.feed.FeedHost
import jp.kz2601.metamon.services.local.RecordSyncEngine
import jp.kz2601.metamon.services.mock.DslSamples
import jp.kz2601.metamon.services.mock.InMemoryAppConfigClient
import jp.kz2601.metamon.services.mock.MockAIClient
import jp.kz2601.metamon.services.mock.MockAuthService
import jp.kz2601.metamon.services.mock.MockSeed
import jp.kz2601.metamon.services.mock.MockTranscriber
import jp.kz2601.metamon.services.mock.OfflineSimulatingAppConfigClient
import jp.kz2601.metamon.services.push.NotifyConsentStore
import jp.kz2601.metamon.services.push.NotifyConsentToggle
import jp.kz2601.metamon.services.push.PushDeviceRegistrar
import jp.kz2601.metamon.services.push.SharedPreferencesPushPrefs
import jp.kz2601.metamon.services.push.forgetAccount
import jp.kz2601.metamon.services.social.MatchingAgeGate
import jp.kz2601.metamon.services.speech.AndroidSpeechTranscriber
import jp.kz2601.metamon.services.sync.HotReload
import jp.kz2601.metamon.services.sync.RealtimeDocumentSync
import jp.kz2601.metamon.services.sync.SseChangeEventSource
import jp.kz2601.metamon.services.sync.asDocumentFetcher
import jp.kz2601.metamon.update.InAppUpdater
import jp.kz2601.metamon.update.PlayAppUpdates
import jp.kz2601.metamon.update.UpdateGate
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import java.io.File
import java.time.Instant

/**
 * 手書きの組み立て 1 か所(#354 §5。Hilt は使わない・iOS の `BuildConfig.make` と同じ形)。
 * モックと本物の差し替えはこのファイルだけで完結する(画面はインターフェースしか知らない)。
 */
class AppContainer(
    val environment: AppEnvironment,
    val auth: AuthService,
    /** クラッシュ報告(Firebase が無ければ何もしない)。オン / オフは [crashReporting] を通す */
    val crashReporter: CrashReporter,
    /** AI へのデータ送信の同意(#888)。[ai] はこれで包んである */
    val consent: AiConsentStore,
    /** 設定の「クラッシュ報告を送る」(#888) */
    val crashReporting: CrashReportingPreference,
    /** aiButton の端末内 AI の集計(#888)。設定の「AI ボタンの利用状況を送る」 */
    val aiButtonMetrics: AiButtonMetricsRecorder,
    /** 集計の送り先(`POST /v1/metrics/ai-button`)。モックは null(送らない) */
    val aiButtonMetricsSender: AiButtonMetricsSender?,
    val initialDsl: AppDSL,
    val appConfig: AppConfigClient,
    val ai: AIClient,
    /** 文字起こし。本物は端末内の認識、モックは決まった文を返す [MockTranscriber] */
    val transcriber: SpeechTranscriber,
    /** 今のアプリの appId(端末に保存・次の起動で開き直す)。切り替え・新しいアプリはランタイムが書き換える */
    val appId: CurrentAppStore,
    /** 最後に取得したアプリの一覧(オフラインの一覧) */
    val appList: AppListCache,
    /** 外部の公開データ(feed・#876)。モックは null(部品は「読み込めませんでした」・iOS と同じ) */
    val feedHost: FeedHost? = null,
    /** [feedHost] の覚えた行(#876)。サインアウト・削除で消す。モックは null */
    private val feedCache: FeedCache? = null,
    val knobs: DebugKnobs,
    /** 記録のローカル DB と送信待ちの列(#882)。[MetamonApplication] が start する */
    val recordSync: RecordSyncEngine,
    /** 最後に取得した DSL(オフライン起動・#883)。appconfig が取得のたびに入れる */
    val documentCache: DocumentCache,
    /** 他端末・共有の相手の変更(SSE + ポーリングの予備・#883)。モックは null */
    val hotReload: HotReload?,
    private val host: RuntimeHost,
    /** 他の人の行動で届く通知(#887)。モック・Firebase の無い構成では null(何もしない) */
    val push: PushHost? = null,
    /** アカウント削除の `DELETE /v1/me`(#841) */
    val account: AccountClient,
    /** 端末のデータの持ち主(起動をまたいで覚える・#841) */
    private val ownerStore: AccountOwnerStore,
    /** トークンを付けてよい uid(#841)。[accountGuard] が書く */
    private val activeOwner: ActiveOwner,
    /** 権限の記録など、端末の設定のうち前の人の分(#841) */
    private val forgetDeviceState: () -> Unit,
    /** 共有アプリへの投稿に出す名前(#875)。設定で決め、サインアウト・削除で消す */
    val authorProfile: AuthorProfile,
    /** アプリごとの会話の履歴(#893)。モックはメモリだけ */
    val chatHistory: ChatHistoryStore,
    /** 端末内の動画(#863)・録音(#1084)のファイルを全部消す(#841 の後片付け) */
    private val removeLocalMedia: () -> Unit,
    /** 仲間探し・通報・ブロック(#877)。モックは null(出さない) */
    private val social: SocialServices? = null,
    /** https の画像のキャッシュを消す(#841 の後片付け) */
    private val clearImageCache: () -> Unit = {},
    /** 記録の保存期間を確認した記録(#482・端末ごと)。モックはメモリだけ */
    private val retentionConsent: RetentionConsentStore = KeyValueRetentionConsent(InMemoryKeyValueStore()),
    /** 初回のチュートリアルと「こう話すと直せます」を見た印(#900)。モックは見た扱いで始める */
    val tutorial: FirstRunTutorialStore = FirstRunTutorialStore(InMemoryKeyValueStore()).apply { markAllSeen() },
    /** TalkBack が動いているか(削除の取り消しの時間を延ばす・H8) */
    private val talkBackRunning: () -> Boolean = { false },
    /** アプリ内のフィードバックの送り先(`POST /v1/feedback`・#319)。モックは送った内容を覚えるだけ */
    val feedback: FeedbackClient = MockFeedbackClient(),
    /** 最低対応版とアプリ内のお知らせ(#906)。モックは何もしない(iOS と同じ) */
    val updateGate: UpdateGate = UpdateGate.disabled(InMemoryKeyValueStore()),
    /** 更新の画面から Play の更新を始める。モック・Play 以外から入れた端末は null の口(ストアのページに落とす) */
    val inAppUpdater: InAppUpdater = InAppUpdater(null),
    /** 「探す」と公開(#902)。モックは null(「サーバー接続時のみ使えます」・iOS と同じ) */
    val discover: DiscoverServices? = null,
    /** 共有アプリの参加者(#903)。モックは null(参加者の欄を出さない) */
    val members: MembersClient? = null,
) {
    /** 共有シートの「みんなに公開」に渡すもの(#902)。「探す」が無い構成は null */
    val publishEnv: PublishEnv? by lazy { discover?.let { PublishEnv(it.client, authorProfile) } }

    /** フィードバックの画面の出し入れ(設定の入口・端末を振った時) */
    val feedbackComposer = FeedbackComposerLauncher()

    /** 画面の下に出す接続先の名前(モックなら "mock")。 */
    val environmentLabel: String get() = if (knobs.mock) "mock" else environment.name

    /** サインイン状態(未サインインならサインイン画面) */
    val session = AuthSession(auth, beforeSignOut = { push?.unregister() })

    /** 同意の画面を出す時点(起動時・AI に送る直前・設定から・#888) */
    val consentPrompt = ConsentPrompt(consent)

    /** 初回のチュートリアルを出す時点(同意の後・設定の「使い方を見る」・#900) */
    val tutorialPrompt = TutorialPrompt(tutorial)

    /** 前の日までの aiButton の集計を送る(起動・前面に戻った時。送るのは 1 日 1 回) */
    suspend fun flushAiButtonMetrics() {
        aiButtonMetricsSender?.let { aiButtonMetrics.flush(it) }
    }

    /** ログアウト・アカウント削除を回す scope(画面が消えても途中で止めない・#841)。 */
    val accountScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    /** 端末のデータの後片付け(#841・iOS の `wipeLocalData`)。 */
    val wipeLocalData: LocalDataWipe by lazy {
        LocalDataWipe(
            recordSync, documentCache, appList, resetRuntime = { runtime.resetForAccountChange() }, forgetDeviceState = forgetDeviceState,
            chatHistory = chatHistory,
            authorProfile = authorProfile,
            removeLocalMedia = removeLocalMedia,
            clearImageCache = clearImageCache,
            feedCache = feedCache,
            closeFeedback = feedbackComposer::close,
            // チュートリアルと「こう話すと直せます」の印(iOS は LocalWipe.clearDefaults で消える)。次の人には出し直す
            forgetTutorial = {
                tutorial.forget()
                tutorialPrompt.reset()
            },
            // 同意は人ごと(iOS は UserDefaults ごと消す)。前の人の止めた発話も次の人の同意で送らない
            forgetConsent = {
                consent.forget()
                consentPrompt.reset()
                aiButtonMetrics.clearCounts()
                social?.ageGate?.reset() // 18 歳以上の確認(#877)も人ごと(iOS は UserDefaults ごと消す)
                runtime.retention?.forget() // 記録の保存期間の確認(#482)も人ごと(iOS は UserDefaults ごと消す)
            },
        )
    }

    /**
     * サインアウト(外からも)・uid が変わった時に必ず後片付けし、記録の同期はサインイン後だけ動かす(#841)。
     * [MetamonApplication] が start する。画面は [AccountGuard.active] で本体を出す。
     */
    val accountGuard: AccountGuard by lazy {
        AccountGuard(
            auth = auth,
            saved = ownerStore,
            owner = activeOwner,
            startSync = recordSync::start,
            stopSync = recordSync::stop,
            wipe = { wipeLocalData() },
            onSignedOut = {
                ShareLinkIntents.inbox.clear()
                feedbackComposer.close() // 消す前に開き直されても、サインアウトで閉じる(#1315)
            },
            onOwnerReplaced = runtime::resumeAfterAccountSwitch,
        )
    }

    companion object {
        /**
         * `knobs.mock` なら Firebase を使わず固定 uid のモック認証・通信しない構成で組む(iOS の `METAMON_MOCK`・#992)。
         */
        fun make(context: Context, knobs: DebugKnobs = DebugKnobs.NONE): AppContainer {
            val environment = AppEnvironment(
                name = BuildConfig.METAMON_ENV,
                aiBaseUrl = BuildConfig.AI_BASE_URL,
                appConfigBaseUrl = BuildConfig.APPCONFIG_BASE_URL,
            )
            val samples = DslSamples { name -> runCatching { context.assets.open("$name.json").bufferedReader().use { it.readText() } }.getOrNull() }
            val initialDsl = loadInitialDsl(samples, knobs)
            return if (knobs.mock) {
                makeMock(context, environment, samples, initialDsl, knobs)
            } else {
                makeLive(context, environment, initialDsl, knobs)
            }
        }

        /** `METAMON_MOCK=1`: 通信しない構成(iOS の `.mock`)。 */
        private fun makeMock(context: Context, environment: AppEnvironment, samples: DslSamples, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            Log.i(TAG, "モックの構成で起動します($knobs)")
            val appConfig = InMemoryAppConfigClient(
                seed = MockSeed.demoRecords(),
                apps = if (knobs.mockSharing) MockSeed.sharingApps else emptyList(),
                sharing = knobs.mockSharing,
                initialDocument = { appId -> samples.document(DslSamples.INITIAL, appId, 1) },
            )
            val delayMillis = knobs.mockAiDelaySeconds?.let { (it * 1000).toLong() } ?: 1_200
            val records = offlineIfRequested(appConfig, knobs)
            // モックは端末に残さない・同意済みで始める(設定から取り消して同意の画面を試せる)
            val prefs: KeyValueStore = InMemoryKeyValueStore()
            val consent = AiConsentStore(prefs).apply { grant() }
            // DSL に書かれた https の画像は外のサーバーなのでモックでも読む
            RemoteImages.install(context, MetamonHttp.defaultClient())
            return AppContainer(
                environment = environment,
                auth = MockAuthService(signedIn = true),
                crashReporter = NoopCrashReporter,
                consent = consent,
                crashReporting = CrashReportingPreference(prefs),
                aiButtonMetrics = AiButtonMetricsRecorder(prefs),
                aiButtonMetricsSender = null,
                initialDsl = initialDsl,
                appConfig = records,
                ai = ConsentGatedAiClient(MockAIClient(samples, delayMillis = delayMillis)) { consent.isGranted },
                transcriber = MockTranscriber(),
                // 本物のアプリ・記録と混ざらないよう、モックは別の置き場
                appId = DeviceAppId.store(context, "metamon-app-mock", migrateLegacy = false),
                appList = AppListCache(File(context.filesDir, "metamon-app-mock")),
                knobs = knobs,
                recordSync = recordSync(context, records, "metamon-records-mock"),
                documentCache = DocumentCache(null),
                hotReload = null,
                host = AndroidRuntimeHost(context.applicationContext),
                account = MockAccountClient,
                ownerStore = AccountOwnerStore(File(context.filesDir, "metamon-account-mock")),
                activeOwner = ActiveOwner(),
                forgetDeviceState = forgetDeviceState(context),
                // モックは端末に残さない(前の起動の名前・履歴を持ち込まない)
                authorProfile = AuthorProfile(InMemoryKeyValueStore()),
                chatHistory = ChatHistoryStore(null),
                removeLocalMedia = removeLocalMedia(context),
                clearImageCache = { RemoteImages.wipe(context) },
                talkBackRunning = { TalkBack.isRunning(context) },
            )
        }

        private fun makeLive(context: Context, environment: AppEnvironment, initialDsl: AppDSL, knobs: DebugKnobs): AppContainer {
            val firebase = FirebaseBootstrap.configureIfAvailable(context, AppCheckProviders.factory())
            val auth = chooseAuth(firebase) { FirebaseAuthService.createIfConfigured() }

            // backend への Bearer は Firebase の ID トークン(モックなら付けない)。
            // 端末のデータの持ち主を受け入れるまでは付けない(前の人の送信待ちを次の人のトークンで送らない・#841)
            val owner = ActiveOwner()
            val tokens = OwnerGatedTokens(auth, owner)
            val clientInfo = ClientInfo(version = BuildConfig.VERSION_NAME, build = BuildConfig.VERSION_CODE.toString())
            // アプリ内で選んだ言語(無ければ端末の言語)。リクエストごとに読むので切り替えはすぐ効く(#843)
            val language = { AppLocales.languageCode(context) }
            val device = DeviceContext(language = language)
            // App Check: release は Play Integrity・debug は debug provider。Firebase が無ければヘッダ無し
            val appCheck = if (firebase) FirebaseAppCheckTokenProvider.createIfConfigured() else null
            val client = MetamonHttp.defaultClient()
            // 画像も同じ接続を使う(トークンは付けない)
            RemoteImages.install(context, client)
            fun http(baseUrl: String) = MetamonHttp(baseUrl, tokens, clientInfo, appCheck = appCheck, client = client, device = device)
            val log: (String) -> Unit = { Log.w(TAG, it) }

            val documentCache = DocumentCache(File(context.filesDir, "metamon-dsl"))
            val appConfigHttp = http(environment.appConfigBaseUrl)
            val appConfig = offlineIfRequested(HttpAppConfigClient(appConfigHttp, cache = documentCache), knobs)
            val sync = RealtimeDocumentSync(appConfig.asDocumentFetcher(), SseChangeEventSource(appConfigHttp), log = log)
            // 同意・クラッシュ報告・AI ボタンの集計の置き場(iOS の UserDefaults の鍵と同じ名前・#888)
            val prefs = SharedPreferencesKeyValueStore.of(context, PRIVACY_PREFS)
            val consent = AiConsentStore(prefs)
            val push = if (firebase) push(context, appConfigHttp, auth, log) else null
            val feedCache = FeedCache(File(context.filesDir, "metamon-feeds"))
            // 投稿の通報・ブロック(#877)と掲載の通報・ブロック(#902)は同じ口
            val moderation = HttpModerationClient(appConfigHttp)

            return AppContainer(
                environment = environment,
                auth = auth,
                crashReporter = (if (firebase) FirebaseCrashReporter.createIfConfigured() else null) ?: NoopCrashReporter,
                consent = consent,
                crashReporting = CrashReportingPreference(prefs),
                aiButtonMetrics = AiButtonMetricsRecorder(prefs),
                aiButtonMetricsSender = HttpAiButtonMetricsSender(appConfigHttp),
                initialDsl = initialDsl,
                appConfig = appConfig,
                // 同意が無ければ一切送らない(最後の砦。画面側でも送る前に止めて同意の画面を出す)
                ai = ConsentGatedAiClient(HttpAIClient(http(environment.aiBaseUrl), capabilities = deviceCapabilities(), log = log, cache = documentCache)) { consent.isGranted },
                // 声は端末内だけで文字にする(#354 §3)。端末内が無い端末は文字入力に落とす
                transcriber = AndroidSpeechTranscriber(context, language = language, log = { Log.i(TAG, it) }),
                appId = DeviceAppId.store(context, "metamon-app"),
                appList = AppListCache(File(context.filesDir, "metamon-app")),
                feedHost = AppFeedHost(HttpFeedClient(appConfigHttp), feedCache),
                feedCache = feedCache,
                knobs = knobs,
                recordSync = recordSync(context, appConfig, "metamon-records"),
                documentCache = documentCache,
                hotReload = HotReload(sync, CoroutineScope(SupervisorJob() + Dispatchers.Default)),
                host = AndroidRuntimeHost(context.applicationContext),
                push = push,
                account = HttpAccountClient(appConfigHttp),
                ownerStore = AccountOwnerStore(File(context.filesDir, "metamon-account")),
                activeOwner = owner,
                forgetDeviceState = forgetDeviceState(context),
                authorProfile = AuthorProfile(SharedPreferencesKeyValueStore.of(context, AUTHOR_PREFS)),
                retentionConsent = KeyValueRetentionConsent(SharedPreferencesKeyValueStore.of(context, RETENTION_PREFS)),
                chatHistory = ChatHistoryStore(File(context.filesDir, "chat_history")),
                removeLocalMedia = removeLocalMedia(context),
                social = SocialServices(
                    HttpMatchingClient(appConfigHttp),
                    moderation,
                    MatchingAgeGate(prefs),
                    follows = HttpFollowsClient(appConfigHttp),
                ),
                clearImageCache = { RemoteImages.wipe(context) },
                tutorial = FirstRunTutorialStore(SharedPreferencesKeyValueStore.of(context, FirstRunTutorialStore.PREFS)),
                talkBackRunning = { TalkBack.isRunning(context) },
                feedback = HttpFeedbackClient(appConfigHttp),
                updateGate = UpdateGate.live(context, environment.appConfigBaseUrl, client, device),
                inAppUpdater = InAppUpdater(PlayAppUpdates.ifInstalledFromPlay(context)),
                discover = DiscoverServices(HttpDiscoverClient(appConfigHttp), moderation),
                members = HttpMembersClient(appConfigHttp),
            )
        }

        private fun push(context: Context, http: MetamonHttp, auth: AuthService, log: (String) -> Unit): PushHost {
            val notifications = LocalNotifications.get(context)
            // FCM が裏で出す通知もこのチャンネルに入るので先に作る
            notifications.ensureChannels()
            val client = HttpPushDeviceClient(http)
            val prefs = SharedPreferencesPushPrefs(FcmTokens.prefs(context))
            val consentStore = NotifyConsentStore(prefs)
            return PushHost(
                registrar = PushDeviceRegistrar(client, prefs, appVersion = BuildConfig.VERSION_NAME, os = FcmTokens.osName(), log = log),
                consentStore = consentStore,
                consent = NotifyConsentToggle(client, consentStore, log = log),
                uid = { auth.currentUser.value?.uid },
                canPost = notifications::canPost,
                fetchToken = FcmTokens::current,
                registerDevices = PUSH_DEVICE_REGISTRATION,
                log = log,
            )
        }

        /**
         * FCM のトークンを backend に送るか。今の `PUT /v1/me/devices` は APNs の 16 進のトークンだけを受ける(platform も無い)ので、
         * backend が android を受けるまで(#918)は送らない。同意(notify-consent)と登録の削除はこれと関係なく動く。
         */
        const val PUSH_DEVICE_REGISTRATION = false

        /**
         * 権限の記録と生成アプリの通知の予約(#886)を忘れる(表示の言語は端末の設定なので残す)。
         * 通知の予約は前の人の記録から作った文面を持ち、残すとサインアウト後・次の人にも鳴る(iOS は removeAll する)。
         */
        private fun forgetDeviceState(context: Context): () -> Unit = {
            PermissionCenter.get(context).forget()
            LocalNotifications.get(context).removeAll()
            // 音声の画面の設定(#892)。iOS は UserDefaults ごと消す(表示の言語などの残すキーには入っていない)
            SpeechSettings(context).clear()
            LaunchVisits.forget(context)
            FabPositionStore(context).clear()
            ClarifySettings(context).clear() // 聞き返しの設定(iOS は UserDefaults ごと消す)
            SharedPreferencesPushPrefs(FcmTokens.prefs(context)).forgetAccount() // 通知の同意の控え(iOS は UserDefaults ごと消す)
            UpdateGate.forgetDismissals(context) // 閉じたお知らせ(iOS は UserDefaults ごと消す)
        }

        /** 端末内の動画・録音・ドラレコのファイルと、撮影・録音の途中で残った一時ファイル。 */
        private fun removeLocalMedia(context: Context): () -> Unit = {
            VideoCodec.store(context).removeAll()
            LocalAudioStore.of(context).removeAll()
            DashcamStore.removeAll(context.filesDir)
            MediaTempFiles.removeAll(context.cacheDir)
        }

        private fun recordSync(context: Context, client: AppConfigClient, dir: String) = RecordSyncEngine.create(
            client = client,
            connectivity = AndroidConnectivity(context),
            directory = File(context.filesDir, dir),
            scope = CoroutineScope(SupervisorJob() + Dispatchers.Default),
        )

        /** `METAMON_E2E_OFFLINE_SECONDS=<n>`: 起動から n 秒は記録の API を圏外にする(iOS `RecordSyncFactory`)。 */
        private fun offlineIfRequested(inner: AppConfigClient, knobs: DebugKnobs): AppConfigClient {
            val seconds = knobs.offlineSeconds ?: return inner
            Log.i(TAG, "起動から ${seconds} 秒は圏外の扱いにします")
            return OfflineSimulatingAppConfigClient(inner, offlineUntil = Instant.now().plusMillis((seconds * 1000).toLong()))
        }

        /** Firebase が使えれば Firebase の認証、使えなければ(json 無し・モックの構成)固定 uid のモック。 */
        internal fun chooseAuth(firebaseConfigured: Boolean, firebase: () -> AuthService?): AuthService =
            (if (firebaseConfigured) firebase() else null) ?: MockAuthService(signedIn = true)

        /**
         * 最初に描く DSL。つまみ(`METAMON_INITIAL_DSL_PATH` > `METAMON_INITIAL_EXAMPLE`)が無ければ
         * 同梱の docs/dsl/examples/initial.json(話しかける前の画面)。`METAMON_INITIAL_SCREEN` で最初の画面を変える。
         */
        internal fun loadInitialDsl(samples: DslSamples, knobs: DebugKnobs): AppDSL {
            val fromKnob = knobs.initialDslPath?.let { path ->
                runCatching { AppDSL.decode(File(path).readText()) }
                    .onSuccess { Log.i(TAG, "DSL override: $path screens=${it.screens.map { s -> s.id }}") }
                    .onFailure { Log.e(TAG, "DSL override failed: $path", it) }
                    .getOrNull()
            } ?: knobs.initialExample?.let { name ->
                val json = samples.json(name)
                if (json == null) Log.e(TAG, "METAMON_INITIAL_EXAMPLE=$name が見つかりません")
                json?.let { DslSamples.envelope("example-$name", 1, it).document.dsl }
            }
            val dsl = fromKnob ?: runCatching { samples.document(DslSamples.INITIAL, "local", 0).document.dsl }.getOrElse {
                Log.e(TAG, "initial.json を読めませんでした", it)
                AppDSL.fallback
            }
            val screen = knobs.initialScreen
            return if (screen != null && dsl.screen(screen) != null) dsl.copy(initialScreen = screen) else dsl
        }

        const val TAG = "Metamon"

        /** 同意・クラッシュ報告・AI ボタンの集計(#888) */
        private const val PRIVACY_PREFS = "metamon-privacy"
        private const val AUTHOR_PREFS = "metamon-author"
        private const val RETENTION_PREFS = "metamon-retention"
    }

    /** 今のアプリの参加者と自分が管理者か(#903)。部品の member の選択肢・管理者の削除が読む */
    val membersStore = AppMembersStore(members, CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate))

    /** アプリ全体のランタイム(#889)。起動時に 1 つだけ作る。 */
    val runtime: AppRuntime by lazy {
        AppRuntime(
            // 前に開いていたアプリの appId で始める。中身は bootstrap が保存した DSL → サーバーの最新の順で当てる
            document = AppDocument(appId = appId.current, dsl = initialDsl),
            recordSync = recordSync,
            scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
            host = host,
            currentUserId = { auth.currentUser.value?.uid },
            restore = { id -> appConfig.fetchDocument(id).document },
            hotReloader = hotReload,
            cachedDocument = { id -> documentCache.load(id)?.document },
            directory = AppDirectory(
                store = appId,
                list = appConfig::listApps,
                cache = appList,
                fresh = { id -> AppDocument(appId = id, dsl = initialDsl) },
            ),
            aiHost = aiHost,
            authorProfile = authorProfile,
            socialServices = social,
            // 後片付けを始めたら(ActiveOwner が外れたら)消さない(#841 の OwnerGatedTokens と同じ)
            retention = RecordRetentionRunner(
                recordSync, retentionConsent,
                currentUserId = { activeOwner.uid?.takeIf { it == auth.currentUser.value?.uid } },
            ),
            undoGraceMillis = { TalkBack.recordUndoMillis(talkBackRunning()) },
            members = membersStore,
        ).also { it.dictation = TranscriberDictationHost(transcriber, CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)) }
    }

    /**
     * aiButton のホスト(#873)。端末内 AI(Gemini Nano)はまだ入れていないので、いつもオンライン版に回る。
     * 同意は [consent] を読み、未同意なら同意の画面を出す(#888)。モックは課金の制限なし。
     */
    private val aiHost: AppAiHost by lazy {
        AppAiHost(
            ai = ai,
            appId = { runtime.document.appId },
            access = { AiTextAccess(consentGranted = consent.isGranted, isPro = false, enforced = !knobs.mock) },
            sharedApp = { runtime.entityModels.any { it.isShared } },
            isMock = knobs.mock,
            metrics = aiButtonMetrics::record,
            onNeedsConsent = { consentPrompt.require() },
        )
    }
}
