import SwiftUI
import Observation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// アプリ全体の単一ランタイム。DSL ドキュメント・レコードキャッシュ・ナビゲーション・アクション実行を担う。
/// Renderer には `DSLRuntime` protocol 越しに見せる(`.environment(\.dslRuntime, runtime)`)。
@MainActor
@Observable
final class AppRuntime: DSLRuntime {
    private(set) var document: AppDocument
    var dsl: AppDSL { document.dsl }

    /// entity id → レコード(サーバーが正。ここはキャッシュ)
    private(set) var records: [String: [Record]] = [:]
    private(set) var loading: Set<String> = []
    /// サーバーから取れず、ローカルにも無い entity(#85)。Renderer は空状態ではなく「読み込めませんでした(再試行)」を描く。
    private(set) var failed: Set<String> = []

    var paths: [String: [Route]] = [:]
    /// タブ表示時に選択中のタブ(= tab.screen)。
    var selectedTab: String = ""
    var sheet: Route?
    var fullScreen: Route?
    var showVoiceSheet = false
    /// 変身一覧シート(#51: FAB 長押し / 「アプリ一覧」)。
    var showAppSwitcher = false
    /// 自分のアプリ一覧(`appsList()` が更新)。
    private(set) var apps: [AppSummary] = []
    private(set) var appsLoading = false
    /// 初期(音声)画面に来た経緯。挨拶文言(`Greeting`)の出し分けに使う。
    enum HomeEntry { case launch, newApp, home }
    var homeEntry: HomeEntry = .launch

    struct Confirm: Identifiable {
        let id = UUID()
        var message: String
        var perform: @MainActor () async -> Void
    }
    var confirm: Confirm?
    /// 軽微な通知(下部カプセル・保存失敗など)。生成の失敗はこちらではなく `errorCard`、生成の summary は `summaryBanner`。
    var toast: String? {
        didSet {
            #if DEBUG
            if let toast { print("[Metamon] toast: \(toast)") } // ヘッドレス E2E でエラー文言を拾うため
            #endif
        }
    }

    /// 生成(morph)まわりのエラー **状態カード**(#60)。音声画面 / 生成後の画面上部に出し、主ボタンで復帰する。nil なら正常。
    var errorCard: ErrorPresentation? {
        didSet {
            #if DEBUG
            if let errorCard { print("[Metamon] errorCard: \(errorCard.kind) transcript=\(errorCard.transcript)") }
            else if oldValue != nil { print("[Metamon] errorCard: cleared") }
            #endif
        }
    }
    /// 401 が再取得後も続いた時(セッション切れ)。`MetamonApp` が `AuthSession.expire()` を結ぶ。
    @ObservationIgnored var onSessionExpired: (@MainActor () -> Void)?
    /// 圏外で送れなかった発話を 1 件保留し、復帰で 1 回だけ送る。
    @ObservationIgnored private let deferredMorph: DeferredSend<String>
    @ObservationIgnored private var connectivityTask: Task<Void, Never>?
    /// 生成中に圏外になって自分でキャンセルした(ユーザーのキャンセルと区別する)
    @ObservationIgnored private var interruptedByOffline = false

    /// 生成中の状態。nil なら生成していない。`create` は初期状態からの生成(全画面演出)、`edit` は 2 回目以降(薄いインジケータ)。
    struct MorphJob: Equatable {
        enum Kind { case create, edit }
        var kind: Kind
        var transcript: String
        var startedAt: Date
    }
    private(set) var morphJob: MorphJob?
    var isMorphing: Bool { morphJob != nil }

    /// 変身直後に画面上部へ数秒出す要約(読み上げ文)。位置ヒントがあれば該当領域をハイライトする。
    struct SummaryBanner: Identifiable, Equatable {
        let id = UUID()
        var text: String
        var highlight: UnitPoint?
    }
    var summaryBanner: SummaryBanner?

    /// サーバー(appconfig)に届かず、ローカルキャッシュの DSL を表示している。
    private(set) var isOffline = false

    let services: Services
    /// レコードのローカル DB + 未送信キュー(Issue #48)。`records` はここから読み、書きはここに即反映してから裏で送る。
    let recordSync: RecordSyncEngine
    @ObservationIgnored private var syncTask: Task<Void, Never>?
    @ObservationIgnored private var morphTask: Task<MorphResult, Error>?
    @ObservationIgnored private var isForeground = true
    /// 「元に戻して」のフォールバック(appconfig が versions を持たない mock / 未デプロイ時)。直近 10 件。
    @ObservationIgnored private var localHistory: [AppDocument] = []

    init(document: AppDocument, services: Services) {
        self.document = document
        self.services = services
        self.recordSync = RecordSyncFactory.make(client: services.appConfig)
        self.deferredMorph = DeferredSend(connectivity: recordSync.connectivity)
        self.selectedTab = document.dsl.initialTab
        recordSync.onEvent = { [weak self] event in self?.handle(event) }
    }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    /// サーバーに届かなければ最後に取得した DSL(`DocumentCache`)を出し、トーストで「オフライン」を知らせる。
    /// 通知の同期(許可ダイアログを出し得る)は復元・同期の **後ろで await せずに** 回す(#86: ダイアログ待ちで復元が止まらない)。
    func bootstrap() async {
        Task { try? await services.transcriber.prepare() }
        recordSync.start()
        observeConnectivity()
        if services.isLive {
            await restoreFromServer()
            startSync()
        }
        Task { await services.notifications.sync(dsl.app.notifications) }
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_E2E_PRESS_PRIMARY_AFTER=<秒>` で、その時点で状態カードが出ていれば主ボタン(もう一度 等)を押す(#60)
        if let raw = ProcessInfo.processInfo.environment["METAMON_E2E_PRESS_PRIMARY_AFTER"], let seconds = Double(raw), seconds > 0 {
            Task { [weak self] in
                try? await Task.sleep(for: .seconds(seconds))
                guard let self, let card = errorCard, let primary = card.primary else { print("[Metamon] E2E: no error card to act on"); return }
                print("[Metamon] E2E: pressing primary \(primary) on \(card.kind)")
                handle(primary, of: card)
            }
        }
        // ヘッドレス E2E 用: `METAMON_AUTO_TRANSCRIPT="家計簿アプリが欲しい"` で起動直後に 1 回だけ生成を投げる
        if let text = ProcessInfo.processInfo.environment["METAMON_AUTO_TRANSCRIPT"], !text.isEmpty {
            await morph(transcript: text)
        }
        // ヘッドレス E2E 用: `METAMON_AUTO_RECORD='{"entity":"expense","data":{"amount":980,...}}'` で起動直後に 1 件保存する
        if let raw = ProcessInfo.processInfo.environment["METAMON_AUTO_RECORD"], let data = raw.data(using: .utf8),
           let spec = try? JSONDecoder().decode(AutoRecord.self, from: data) {
            records[spec.entity] = await recordSync.save(appId: document.appId, entity: spec.entity, recordId: nil, data: spec.data)
            print("[Metamon] auto record saved locally: entity=\(spec.entity) count=\(records[spec.entity]?.count ?? 0) sync=\(recordSync.status.label)")
        }
        #endif
    }

    #if DEBUG
    private struct AutoRecord: Decodable {
        var entity: String
        var data: [String: JSONValue]
    }
    #endif

    /// `GET /dsl` で復元。サーバーの version が今より進んでいれば差し替え、遅れていれば(ai の store だけ進んだ等)今の表示を保つ。
    private func restoreFromServer() async {
        do {
            let remote = try await services.appConfig.fetchDocument(appId: document.appId)
            isOffline = false
            if remote.version > document.version || (remote.version == document.version && document.version == 0) {
                apply(remote, animated: false)
            }
            print("[Metamon] restore: server version=\(remote.version) shown=\(document.version) screens=\(dsl.screens.map(\.id))")
        } catch {
            print("[Metamon] restore failed: \(error)")
            if let cached = DocumentCache.shared.load(appId: document.appId), cached.version >= document.version {
                apply(cached, animated: false)
                isOffline = true
                toast = ErrorCopy.offlineRestored(version: cached.version)
            } else {
                isOffline = true
                toast = ErrorCopy.unreachable(error.localizedDescription)
            }
        }
    }

    /// 到達性を見張る(#60): 生成中に圏外になったら即キャンセルして「通信が切れました」、復帰したら未取得の entity を読み直す。
    private func observeConnectivity() {
        connectivityTask?.cancel()
        connectivityTask = Task { [weak self] in
            guard let self else { return }
            var previous: Bool?
            for await online in recordSync.connectivity.updates() {
                defer { previous = online }
                if !online, morphJob != nil, morphTask != nil {
                    interruptedByOffline = true
                    morphTask?.cancel()
                }
                if online, previous == false {
                    let retry = failed
                    for entity in retry { await refresh(entity) }
                    if isOffline, services.isLive { await restoreFromServer() }
                }
            }
        }
    }

    func startSync() {
        syncTask?.cancel()
        guard services.isLive, isForeground else { return }
        let appId = document.appId
        let version = document.version
        syncTask = Task { [weak self] in
            guard let self else { return }
            for await doc in services.sync.updates(appId: appId, after: version) {
                guard doc.version > document.version, morphJob == nil else { continue }
                print("[Metamon] hot reload: version \(document.version) → \(doc.version)")
                apply(doc)
                summaryBanner = SummaryBanner(text: "アプリが更新されました(version \(doc.version))", highlight: nil)
            }
        }
    }

    /// 前面の時だけポーリングする(G13 Day1 版)。背面では止め、戻ったら今の version から張り直す。
    func setForeground(_ active: Bool) {
        guard isForeground != active else { return }
        isForeground = active
        recordSync.setForeground(active)
        if active { startSync() } else { syncTask?.cancel() }
    }

    /// 新しい DSL に差し替える。ナビ・キャッシュは初期化(画面構成が変わるため)。通知は DSL に同期。
    func apply(_ doc: AppDocument, animated: Bool = true) {
        let swap = {
            self.document = doc
            self.records = [:]
            self.failed = []
            self.errorCard = nil
            self.paths = [:]
            self.sheet = nil
            self.fullScreen = nil
            self.showVoiceSheet = false
            self.selectedTab = doc.dsl.initialTab
            self.morphJob = nil
        }
        if animated {
            withAnimation(.spring(duration: 0.65, bounce: 0.12), swap)
        } else {
            swap()
        }
        Task { await services.notifications.sync(doc.dsl.app.notifications) }
        WidgetSnapshotWriter.write(dsl: doc.dsl, records: records) // ホーム画面ウィジェットへ stat を反映
    }

    // MARK: - Morph / Undo / New app

    /// 発話 → ai サービス → 最新 DSL 適用。
    /// 送る前に iOS 側で意図を判定する: 「元に戻して」系は appconfig の restore(ai を呼ばない)、
    /// 「別のアプリ」系は appId を採番し直してから ai へ(前のアプリはサーバーに残る)。
    /// 生成中は `morphJob` が立ち、RootView が create(全画面「変身中」)/ edit(薄いインジケータ)を出す。
    /// 完了時は新ツリーの挿入と生成中オーバーレイの消去を**同じアニメーション**で行い、クロスフェードにする。
    func morph(transcript: String) async {
        let text = transcript.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, morphJob == nil else { return }
        switch VoiceIntent.detect(text) {
        case .undo:
            await undo()
            return
        case .newApp:
            startNewApp()
        case .listApps:
            openSwitcher(afterSheetDismiss: !dsl.isVoiceOnly)
            return
        case .switchApp(let name):
            await switchApp(named: name)
            return
        case .morph:
            break
        }
        // 圏外(#60): 送らずに保留し、状態カード「接続が戻ったら自動で送ります」。復帰で 1 回だけ自動送信(モックは常にオンライン)
        if services.isLive || recordSync.connectivity is ManualConnectivity,
           deferredMorph.holdIfOffline(text, send: { [weak self] queued in await self?.sendDeferred(queued) }) {
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation(kind: .offlineQueued, transcript: text) }
            return
        }
        await send(text)
    }

    /// 復帰時の自動送信(`DeferredSend` から)。保留が生きていれば生成に進む。
    private func sendDeferred(_ text: String) async {
        guard errorCard?.kind == .offlineQueued, errorCard?.transcript == text else { return }
        print("[Metamon] back online → sending deferred transcript")
        await send(text)
    }

    /// ai に送って適用する本体。失敗は `FailureKind` で分類して状態カードに(トーストは使わない)。
    private func send(_ text: String) async {
        guard morphJob == nil else { return }
        withAnimation(.easeInOut(duration: 0.35)) {
            errorCard = nil
            morphJob = MorphJob(kind: dsl.isVoiceOnly ? .create : .edit, transcript: text, startedAt: .now)
        }
        interruptedByOffline = false
        let previous = document
        let task = Task { [services, appId = document.appId, version = document.version] in
            try await services.ai.morph(transcript: text, appId: appId, currentVersion: version)
        }
        morphTask = task
        defer { morphTask = nil }
        do {
            let result = try await task.value
            remember(previous)
            apply(result.document) // withAnimation の中で morphJob = nil も行う
            startSync()
            if let summary = result.summary?.trimmingCharacters(in: .whitespacesAndNewlines), !summary.isEmpty {
                summaryBanner = SummaryBanner(text: summary, highlight: PositionHint.anchor(in: summary))
            }
            print("[Metamon] morph ok: version=\(result.document.version) mode=\(result.mode ?? "-") screens=\(result.document.dsl.screens.map(\.id)) summary=\(result.summary ?? "")")
        } catch {
            let failure: FailureKind = interruptedByOffline ? .offline : FailureKind.classify(error)
            interruptedByOffline = false
            print("[Metamon] morph failed: \(failure) (\(error))")
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            await present(failure, transcript: text)
        }
    }

    /// 失敗の分類ごとに「次の一手」を出す。conflict は最新を取り直して短いバナー、cancelled は軽微なのでトースト、それ以外は状態カード。
    private func present(_ failure: FailureKind, transcript: String) async {
        switch failure {
        case .cancelled:
            toast = ErrorCopy.cancelledToast
        case .conflict:
            // appconfig の version 競合(ai 側の baseVersion 無しリトライでも解けなかった)。最新 DSL を取り直して再適用
            if services.isLive, let latest = try? await services.appConfig.fetchDocument(appId: document.appId) {
                if latest.version != document.version { apply(latest) }
                startSync()
            }
            summaryBanner = SummaryBanner(text: ErrorCopy.conflictBanner, highlight: nil)
        case .sessionExpired:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation.from(failure, transcript: transcript) }
            onSessionExpired?()
        default:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = ErrorPresentation.from(failure, transcript: transcript) }
        }
    }

    /// 状態カードの主ボタン。もう一度 / 今すぐ送る = 同じ発話で再送、サインイン = セッション切れ処理。
    func handle(_ primary: ErrorPresentation.Primary, of card: ErrorPresentation) {
        switch primary {
        case .retry, .sendNow:
            deferredMorph.cancel()
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
            Task { await send(card.transcript) }
        case .signIn:
            withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
            onSessionExpired?()
        }
    }

    /// 状態カードの副ボタン。やめる = 保留を取り消す、閉じる = カードを消す(発話は残らない)。
    func handle(_ secondary: ErrorPresentation.Secondary, of card: ErrorPresentation) {
        if case .discard = secondary { deferredMorph.cancel() }
        withAnimation(.easeInOut(duration: 0.3)) { errorCard = nil }
    }

    /// 生成の中断(30 秒超の長引きで UI から呼ぶ)。URLSession は Task キャンセルで切れる。
    func cancelMorph() {
        morphTask?.cancel()
    }

    /// 「元に戻して」(G11)。appconfig の `GET /versions` で直前の version を取り `POST /versions/{n}/restore`。
    /// サーバーが履歴を返せない時(mock / 未デプロイ)はローカル履歴にフォールバックする。
    func undo() async {
        guard morphJob == nil else { return }
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: "元に戻して", startedAt: .now)
        }
        if services.isLive {
            do {
                let versions = try await services.appConfig.listVersions(appId: document.appId)
                guard let current = versions.first else { throw APIError.server(status: 404, code: "not_found", message: "no versions") }
                guard let target = versions.first(where: { $0.version < current.version }) else {
                    withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
                    toast = "これ以上前の状態はありません"
                    return
                }
                let doc = try await services.appConfig.restore(appId: document.appId, toVersion: target.version)
                apply(doc)
                startSync()
                summaryBanner = SummaryBanner(text: "ひとつ前の状態(version \(target.version))に戻しました", highlight: nil)
                print("[Metamon] undo ok: restored from version \(target.version) → new version \(doc.version)")
                return
            } catch {
                print("[Metamon] undo via server failed, falling back to local history: \(error)")
            }
        }
        guard let previous = localHistory.popLast() else {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "これ以上前の状態はありません"
            return
        }
        apply(previous)
        summaryBanner = SummaryBanner(text: "ひとつ前の状態(version \(previous.version))に戻しました", highlight: nil)
    }

    private func remember(_ doc: AppDocument) {
        localHistory.append(doc)
        if localHistory.count > 10 { localHistory.removeFirst() }
    }

    /// 「別のアプリ。」: appId を新規採番して初期状態にする(サーバー上の前のアプリは残る)。続けて ai に送る前提。
    /// 変身一覧の「+ 新しいアプリ」「メタモンに戻る」からも呼ぶ(`entry` で挨拶を出し分け・アニメ付き)。
    func startNewApp(entry: HomeEntry = .newApp, animated: Bool = false) {
        syncTask?.cancel()
        localHistory.removeAll()
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        homeEntry = entry
        apply(fresh, animated: animated)
        print("[Metamon] new app: appId=\(fresh.appId) entry=\(entry)")
    }

    // MARK: - Switcher (#51 複数アプリの切替)

    /// 変身一覧シートを開く(FAB 長押し / 「アプリ一覧」)。開く直前に今の画面をサムネに撮る。
    /// 音声シートが出ている(いた)時は閉じ終わるのを待ってから撮って開く(シート越しのサムネ・二重 present を避ける)。
    func openSwitcher(afterSheetDismiss: Bool = false) {
        guard !showAppSwitcher else { return }
        let waitForSheet = showVoiceSheet || afterSheetDismiss
        showVoiceSheet = false
        Task {
            if waitForSheet { try? await Task.sleep(for: .milliseconds(500)) }
            AppThumbnailStore.shared.capture(appId: document.appId, isVoiceOnly: dsl.isVoiceOnly)
            showAppSwitcher = true
        }
    }

    /// `GET /v1/apps`。失敗したら `AppListCache`(オフライン)に落ちる。結果は `apps` にも残す。
    @discardableResult
    func appsList() async -> [AppSummary] {
        appsLoading = apps.isEmpty
        defer { appsLoading = false }
        do {
            apps = try await services.appConfig.listApps()
        } catch {
            print("[Metamon] listApps failed: \(error)")
            let cached = AppListCache.shared.load()
            if !cached.isEmpty {
                apps = cached
                toast = "オフライン: 保存済みのアプリ一覧を表示しています"
            } else if apps.isEmpty {
                toast = "アプリ一覧を取得できませんでした: \(error.localizedDescription)"
            }
        }
        return apps
    }

    /// 一覧に出すアプリ(archived・未生成を除き、現在のアプリを先頭に)。
    var visibleApps: [AppSummary] { AppSummary.visible(apps, current: document.appId) }

    /// 別のアプリに切り替える: 今の画面をサムネに撮り → `DeviceAppID` を差し替え → `GET /dsl` → `apply()`(変身アニメ)。
    /// 記録(records)は `apply()` で破棄され、画面が要求する entity を `load(entities:)` の既存経路で読み直す。
    func switchApp(to appId: String, afterSheetDismiss: Bool = false) async {
        let fromSwitcher = showAppSwitcher // 一覧から来たなら開く直前に撮ってある(今撮るとシートが写る)
        showAppSwitcher = false
        guard morphJob == nil, appId != document.appId else { return }
        if !fromSwitcher {
            if afterSheetDismiss { try? await Task.sleep(for: .milliseconds(500)) }
            AppThumbnailStore.shared.capture(appId: document.appId, isVoiceOnly: dsl.isVoiceOnly)
        }
        let previousId = document.appId
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: .edit, transcript: "アプリを切り替えています…", startedAt: .now)
        }
        syncTask?.cancel()
        var next: AppDocument?
        do {
            next = try await services.appConfig.fetchDocument(appId: appId)
            isOffline = false
        } catch {
            print("[Metamon] switch fetch failed: \(error)")
            next = DocumentCache.shared.load(appId: appId)
            if next != nil { isOffline = true }
        }
        guard var doc = next else {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "アプリを読み込めませんでした。接続を確認してください"
            startSync()
            return
        }
        doc.appId = appId
        DeviceAppID.set(appId)
        localHistory.removeAll()
        apply(doc)
        startSync()
        // recordSync(オフライン DB)は呼び出しごとに appId を受けるので再起動不要。旧 appId の未送信はキューが引き続き送り、
        // 新 appId のレコードは refresh() がローカル先出し → サーバー pull で読み直す(イベントは document.appId で絞られる)
        summaryBanner = SummaryBanner(text: "「\(doc.dsl.app.name)」に変身しました" + (isOffline ? "(オフライン・キャッシュ)" : ""), highlight: nil)
        print("[Metamon] switched app: \(previousId) → \(appId) version=\(doc.version)")
    }

    /// 音声「○○に切り替えて」: `GET /apps` の name と照合。1 件に決まれば切替、曖昧/無ければ一覧を開く。
    func switchApp(named name: String) async {
        let fromVoiceSheet = !dsl.isVoiceOnly // 生成後の音声シートからの発話(シートは閉じ始めている)
        await appsList()
        let candidates = visibleApps
        switch AppNameMatch.match(name, in: candidates.map(\.name)) {
        case .one(let i):
            let target = candidates[i]
            if target.id == document.appId {
                summaryBanner = SummaryBanner(text: "いま使っているのが「\(target.name)」です", highlight: nil)
                return
            }
            await switchApp(to: target.id, afterSheetDismiss: fromVoiceSheet)
        case .ambiguous:
            toast = "「\(name)」に近いアプリが複数あります。一覧から選んでください"
            openSwitcher(afterSheetDismiss: fromVoiceSheet)
        case .none:
            toast = "「\(name)」というアプリは見つかりませんでした"
            openSwitcher(afterSheetDismiss: fromVoiceSheet)
        }
    }

    /// 変身一覧の「+ 新しいアプリ」(`.newApp`)と「メタモンに戻る(初期画面)」(`.home`)。
    /// どちらも appId を作り直して初期(音声)画面へ。前のアプリはサーバーと一覧に残り、いつでも戻せる(サムネは一覧を開く時に撮ってある)。
    func startFresh(_ entry: HomeEntry) {
        showAppSwitcher = false
        startNewApp(entry: entry, animated: true)
        summaryBanner = nil
    }

    /// アプリを初期状態(音声画面のみ)に戻す。appId を作り直すので ai/appconfig 側からも「新規端末」に見える(旧データは削除しない)。
    func resetToInitial() {
        syncTask?.cancel()
        localHistory.removeAll()
        DocumentCache.shared.remove(appId: document.appId)
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
        homeEntry = .home
        apply(fresh)
        summaryBanner = nil
        toast = "初期状態に戻しました"
    }

    // MARK: - Records

    func records(for entity: String) -> [Record] { records[entity] ?? [] }

    func load(entities: Set<String>, force: Bool = false) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    /// ローカル先出し → サーバー全件で置換 → UI 更新。サーバーに届かなければローカル表示のまま(通信断は toast しない)。
    func refresh(_ entity: String) async {
        let appId = document.appId
        if records[entity] == nil {
            let cached = await recordSync.cached(appId: appId, entity: entity)
            if !cached.isEmpty, document.appId == appId {
                records[entity] = cached
                WidgetSnapshotWriter.write(dsl: dsl, records: records) // ローカル先出し分もウィジェットへ
            }
        }
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            let fresh = try await recordSync.pull(appId: appId, entity: entity)
            guard document.appId == appId else { return }
            records[entity] = fresh
            failed.remove(entity)
            print("[Metamon] records loaded: entity=\(entity) count=\(fresh.count)")
            WidgetSnapshotWriter.write(dsl: dsl, records: records) // ホーム画面ウィジェットへ stat を反映
        } catch {
            guard document.appId == appId else { return }
            // ローカルにも無ければ「読み込めませんでした」(#85: 空状態と区別する)。ローカルがあればそのまま見せる
            if records[entity]?.isEmpty ?? true {
                records[entity] = []
                failed.insert(entity)
            }
            print("[Metamon] records pull failed (showing local): entity=\(entity) \(error)")
            // 通信断は画面内の placeholder に任せ、トーストは 4xx(権限・入力不備)だけ
            if case .rejected = FailureKind.classify(error) { toast = message(for: error, entity: entity) }
        }
    }

    /// 同期キューからの通知。送信完了はローカル DB を読み直し、競合はサーバーから引き直す(サーバー優先)。
    private func handle(_ event: SyncEvent) {
        Task { @MainActor in
            switch event {
            case .sent(let appId, let entity):
                guard appId == document.appId else { return }
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .conflict(let appId, let entity, _):
                guard appId == document.appId else { return }
                toast = "他の端末で変更されていたため、サーバーの内容に合わせました"
                await refresh(entity)
            case .rejected(let appId, let entity, let message):
                guard appId == document.appId else { return }
                toast = "保存できませんでした: \(message)"
                records[entity] = await recordSync.cached(appId: appId, entity: entity)
                WidgetSnapshotWriter.write(dsl: dsl, records: records)
            case .paused(let pending):
                toast = "送信に失敗しました(未送信 \(pending) 件)。接続を確認してください"
            case .drained:
                break
            }
        }
    }

    // MARK: - Navigation

    func pathBinding(_ stack: String) -> Binding<[Route]> {
        Binding(get: { self.paths[stack] ?? [] }, set: { self.paths[stack] = $0 })
    }

    private func navigate(to screen: String, presentation: Presentation?, recordId: String?, from route: Route) {
        switch presentation ?? .push {
        case .push:
            paths[route.stack, default: []].append(Route(screen: screen, recordId: recordId, stack: route.stack))
        case .sheet:
            sheet = Route(screen: screen, recordId: recordId, stack: "sheet")
        case .fullScreen:
            fullScreen = Route(screen: screen, recordId: recordId, stack: "full")
        }
    }

    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss。
    func close(_ route: Route) {
        if !(paths[route.stack] ?? []).isEmpty {
            paths[route.stack]?.removeLast()
            return
        }
        switch route.stack {
        case "sheet": sheet = nil
        case "full": fullScreen = nil
        default: break // main / tab の根は閉じない
        }
    }

    // MARK: - Actions

    func run(_ action: Action, _ ctx: ActionContext) async {
        if let message = action.confirmMessage {
            confirm = Confirm(message: message) { [weak self] in await self?.perform(action, ctx) }
            return
        }
        await perform(action, ctx)
    }

    private func perform(_ action: Action, _ ctx: ActionContext) async {
        switch action.type {
        case .navigate:
            guard let screen = action.screen else { return }
            navigate(to: screen, presentation: action.presentation, recordId: ctx.recordId, from: ctx.route)

        case .back:
            close(ctx.route)

        case .saveRecord:
            guard let form = ctx.form, let entity = action.entity ?? ctx.form?.entity.id else {
                toast = "保存するフォームがありません"
                return
            }
            let missing = form.missingRequired
            guard missing.isEmpty else {
                toast = "\(missing.joined(separator: "・")) を入力してください"
                return
            }
            // 楽観的 UI: ローカル DB に即反映して画面を進める。appconfig への送信は recordSync のキューが行う
            records[entity] = await recordSync.save(appId: document.appId, entity: entity, recordId: form.recordId, data: form.payload)
            WidgetSnapshotWriter.write(dsl: dsl, records: records)
            if let screen = action.screen {
                navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
            } else {
                close(ctx.route)
            }

        case .deleteRecord:
            guard let entity = action.entity ?? ctx.form?.entity.id,
                  let id = ctx.recordId ?? ctx.form?.recordId else { return }
            records[entity] = await recordSync.delete(appId: document.appId, entity: entity, recordId: id)
            WidgetSnapshotWriter.write(dsl: dsl, records: records)
            if ctx.recordId == nil { close(ctx.route) } // フォーム上からの削除は画面を閉じる

        case .openVoice:
            showVoiceSheet = true

        case .openUrl:
            guard let s = action.url, let url = URL(string: s) else { return }
            _ = await UIApplication.shared.open(url)

        case .resetForm:
            ctx.form?.reset()

        case .scheduleNotification:
            guard let spec = action.notification else { return }
            await attempt {
                try await services.notifications.schedule(spec)
                toast = "通知を予約しました(\(spec.time ?? ""))"
            }

        case .cancelNotification:
            guard let id = action.notificationId else { return }
            services.notifications.cancel(id: id)

        case .unknown:
            toast = "未対応のアクション: \(action.rawType)"
        }
    }

    private func attempt(entity: String? = nil, _ body: () async throws -> Void) async {
        do { try await body() } catch { toast = message(for: error, entity: entity) }
    }

    /// appconfig のエラー封筒を人向けに。422 `invalid_record` は `details.field` を DSL の Field 名に引き直して「どの項目か」を出す。
    func message(for error: Error, entity: String?) -> String {
        guard let api = error as? APIError else { return error.localizedDescription }
        if api.code == "invalid_record" || (api.status == 400 && api.field != nil), let fieldId = api.field {
            let name = entity.flatMap { dsl.entity($0)?.field(fieldId)?.name } ?? fieldId
            return "「\(name)」の入力を確認してください(\(api.errorDescription ?? ""))"
        }
        if api.status == 401 { return ErrorCopy.sessionExpiredSignIn }
        return api.errorDescription ?? "\(api)"
    }
}
