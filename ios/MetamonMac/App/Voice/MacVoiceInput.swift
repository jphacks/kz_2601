import Foundation
import Observation
import MetamonRenderer
import MetamonServices

/// Mac の声で頼む(#966)。マイクは 1 つなのでアプリで 1 つだけ持ち、どのウインドウの会話のシートからも使う。
/// 認識は iOS と同じ `FallbackTranscriber`: 本命は WhisperKit(端末内・CoreML)、準備中は Apple の端末内の認識(SFSpeechRecognizer の
/// requiresOnDeviceRecognition)。声も文字にした内容もサーバーへは送らず、ログにも出さない(文字にした文は会話に入れて送るだけ)。
/// 押して録音 → もう一度押して止める → 文字にする。モデルの取得は `VoiceStartPlan`(Intel の Mac では先に Apple の認識を使う)。
@MainActor
@Observable
final class MacVoiceInput {
    enum State: Equatable {
        case idle
        case recording
        case transcribing
    }

    private(set) var state: State = .idle
    /// 直前の失敗(会話の下に出す)。次に押した時に消える
    private(set) var failure: String?
    /// マイク / 音声認識の許可が無い時、システム設定のどの欄を開くか
    private(set) var settingsPane: MacVoiceCopy.PrivacyPane?
    /// ダウンロードの確認を出すか(シートの alert が読む)
    var showsDownloadConfirm = false
    /// 生成したアプリの話して入力(aiChat・transcribeField)から出す確認か(中央の列の alert が読む・#1328)
    var showsAppDownloadConfirm = false

    @ObservationIgnored let transcriber: any SpeechTranscriber
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private let slowHardware: Bool
    @ObservationIgnored private let isExpensiveNetwork: () -> Bool
    /// この起動の間にダウンロードを一度聞いたか(「あとで」の後は聞き直さない)
    @ObservationIgnored private var askedThisLaunch = false
    /// 声で頼んだ時の確認か(確認の後に録音を続ける)。案内のボタンから開いた時は false
    @ObservationIgnored private var recordsAfterConfirm = false
    /// 録音の回。`cancel()` で進め、待っている間に捨てられた録音・文字を後から使わない
    @ObservationIgnored private var attempt = 0

    init(transcriber: (any SpeechTranscriber)? = nil,
         defaults: UserDefaults = .standard,
         slowHardware: Bool = !MacHardware.isAppleSilicon,
         isExpensiveNetwork: @escaping () -> Bool = { ExpensiveNetworkProbe.shared.isExpensive }) {
        self.transcriber = transcriber ?? Self.makeTranscriber()
        self.defaults = defaults
        self.slowHardware = slowHardware
        self.isExpensiveNetwork = isExpensiveNetwork
    }

    /// 本命は WhisperKit(モデル名は iOS と同じ `MetamonWhisperModel` / `METAMON_WHISPER_MODEL`・既定 small)、準備中は Apple の端末内の認識。
    static func makeTranscriber() -> any SpeechTranscriber {
        let recognizer = WhisperKitRecognizer(model: whisperModel)
        recognizer.language = { LanguagePreference.shared.code } // 認識の言語は表示の言語に合わせる(#45)
        let provisional = AppleOnDeviceTranscriber(language: { LanguagePreference.shared.code })
        return FallbackTranscriber(primary: recognizer, provisional: provisional)
    }

    nonisolated static var whisperModel: String {
        let env = ProcessInfo.processInfo.environment["METAMON_WHISPER_MODEL"]
        let plist = Bundle.main.object(forInfoDictionaryKey: "MetamonWhisperModel") as? String
        return [env, plist].compactMap { $0 }.first { !$0.isEmpty && !$0.hasPrefix("$(") } ?? "small"
    }

    nonisolated static var modelMegabytes: Int { ModelDownloadConsent.approximateMegabytes(model: whisperModel) }

    var isRecording: Bool { state == .recording }
    var isBusy: Bool { state != .idle }
    /// 本命の準備中で Apple の端末内の認識を使う(使っている)
    var isProvisional: Bool { transcriber.isProvisional }
    var status: TranscriberStatus { transcriber.status }
    var downloadProgress: Double { transcriber.downloadProgress }
    /// 遅い見込みの Mac(確認の文に一言足す)
    var isSlowHardware: Bool { slowHardware }

    /// 生成したアプリの話して入力(#1328)。同じ認識を使い、録音は部品が始めた分だけを持つ(声で頼むと取り合わない)
    @ObservationIgnored private(set) lazy var dictation = TranscriberDictation(
        transcriber: transcriber,
        voiceInputActive: { [weak self] in self?.isBusy ?? false },
        stopSpeech: { LeafSpeech.shared.stopForRecording() },
        resumeSpeech: { LeafSpeech.shared.recordingFinished() },
        failure: { MacVoiceCopy.dictationFailure($0) },
        requestModel: { [weak self] in self?.requestDownloadFromApp() })

    // MARK: - 起動

    /// 起動時: モデルが端末にあれば読み込む・承認済みなら取得を続ける。無くて未承認なら起動では聞かない(iOS の M14 と同じ)。
    func prepareIfAllowed() {
        switch ModelDownloadConsent.decide(needsDownload: transcriber.needsDownload,
                                           isExpensiveNetwork: isExpensiveNetwork(), defaults: defaults) {
        case .prepare: Task { try? await transcriber.prepare() }
        case .ask: break
        }
    }

    // MARK: - 録音

    /// 声で頼む(⇧⌘M・マイクのボタン)。録音していなければ始め、録音中なら止めて文字にし、`deliver` に渡す。
    func toggle(deliver: @escaping (String) -> Void) {
        switch state {
        case .idle: begin()
        case .recording: Task { await finish(deliver: deliver) }
        case .transcribing: break
        }
    }

    private func begin() {
        dictation.cancel() // 部品が録音していたら捨てる(マイクは 1 つ)
        failure = nil
        settingsPane = nil
        if case .failed = transcriber.status { Task { try? await transcriber.prepare() } } // 失敗後は作り直す
        let plan = VoiceStartPlan.decide(
            needsDownload: transcriber.needsDownload && transcriber.status == .notDownloaded,
            isApproved: ModelDownloadConsent.isApproved(defaults),
            isExpensiveNetwork: isExpensiveNetwork(),
            slowHardware: slowHardware,
            provisionalAvailable: provisionalAvailable,
            askedThisLaunch: askedThisLaunch)
        switch plan {
        case .record:
            Task { await start() }
        case .askDownload:
            askedThisLaunch = true
            recordsAfterConfirm = true
            showsDownloadConfirm = true
        case .textOnly:
            failure = MacVoiceCopy.textFirst
        }
    }

    private var provisionalAvailable: Bool {
        (transcriber as? FallbackTranscriber)?.provisional.isAvailable ?? false
    }

    /// 確認で「ダウンロード」。取得を始め、仮の聞き取りが使えればそのまま話してもらう
    func approveDownload() {
        ModelDownloadConsent.approve(defaults)
        showsDownloadConfirm = false
        showsAppDownloadConfirm = false
        Task { try? await transcriber.prepare() }
        continueAfterConfirm(downloading: true)
    }

    /// 確認で「あとで」
    func declineDownload() {
        showsDownloadConfirm = false
        showsAppDownloadConfirm = false
        continueAfterConfirm(downloading: false)
    }

    /// 文字で頼んでいる人が、あとから案内のボタンでモデルを入れる時
    func requestDownload() {
        askedThisLaunch = true
        recordsAfterConfirm = false
        showsDownloadConfirm = true
    }

    /// 生成したアプリの話して入力で「音声モデルを用意する」: 承認済みなら取得を始め、まだなら中央の列で確認する
    func requestDownloadFromApp() {
        if ModelDownloadConsent.isApproved(defaults) {
            Task { try? await transcriber.prepare() }
            return
        }
        recordsAfterConfirm = false
        showsAppDownloadConfirm = true
    }

    /// 確認の後: 仮の聞き取りが使えればそのまま録音する。使えなければ、取得中は進み具合を、「あとで」なら文字で頼む案内を出す
    private func continueAfterConfirm(downloading: Bool) {
        guard recordsAfterConfirm else { return }
        recordsAfterConfirm = false
        if provisionalAvailable {
            Task { await start() }
        } else if !downloading {
            failure = MacVoiceCopy.textFirst
        }
    }

    private func start() async {
        guard state == .idle else { return }
        MacSpeechReader.shared.stop() // 読み上げの声をマイクが拾わない(#1210)
        state = .recording
        let current = attempt
        do {
            try await transcriber.startRecording()
            // マイクの許可を待つ間に閉じられた(cancel 済み): 始まったマイクをすぐ止める
            guard current == attempt else { transcriber.cancelRecording(); return }
            MacVoiceOver.announce(MacVoiceCopy.listening)
        } catch {
            guard current == attempt else { return }
            state = .idle
            report(error)
        }
    }

    private func finish(deliver: (String) -> Void) async {
        guard state == .recording else { return }
        state = .transcribing
        let current = attempt
        defer { if current == attempt { state = .idle } }
        do {
            let text = try await transcriber.stopAndTranscribe().trimmingCharacters(in: .whitespacesAndNewlines)
            // 文字にしている間に閉じられた(cancel 済み): 閉じた会話に送らない
            guard current == attempt else { return }
            guard !text.isEmpty else {
                failure = L10n.tr("音声を聞き取れませんでした。もう一度はっきり話してください")
                return
            }
            deliver(text)
        } catch {
            guard current == attempt else { return }
            report(error)
        }
    }

    /// 録音を捨てる(シートを閉じた時など)。録音の開始待ち・文字にしている途中も捨てる
    func cancel() {
        recordsAfterConfirm = false
        guard state != .idle else { return }
        attempt += 1
        if state == .recording { transcriber.cancelRecording() }
        state = .idle
    }

    private func report(_ error: Error) {
        guard let failure = error as? TranscriberFailure else {
            self.failure = L10n.tr("録音できませんでした。もう一度お試しください")
            return
        }
        self.failure = MacVoiceCopy.message(failure)
        settingsPane = MacVoiceCopy.privacyPane(for: failure)
    }
}
