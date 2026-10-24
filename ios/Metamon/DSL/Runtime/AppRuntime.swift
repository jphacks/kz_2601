import SwiftUI
import Observation

/// NavigationStack の path 要素。画面 id + 編集対象 recordId + どのスタック(main / sheet / full)に属するか。
struct Route: Hashable, Identifiable {
    var screen: String
    var recordId: String?
    var stack: String = "main"
    var id: String { "\(stack)/\(screen)/\(recordId ?? "")" }
}

/// アクション実行時の文脈(どの画面・どのフォーム・どのレコードから起動されたか)。
struct ActionContext {
    var route: Route
    var form: FormDraft?
    /// list の行タップ時に iOS が暗黙に渡す recordId
    var recordId: String?
}

/// アプリ全体の単一ランタイム。DSL ドキュメント・レコードキャッシュ・ナビゲーション・アクション実行を担う。
@MainActor
@Observable
final class AppRuntime {
    private(set) var document: AppDocument
    var dsl: AppDSL { document.dsl }

    /// entity id → レコード(サーバーが正。ここはキャッシュ)
    private(set) var records: [String: [Record]] = [:]
    private(set) var loading: Set<String> = []

    var paths: [String: [Route]] = [:]
    /// タブ表示時に選択中のタブ(= tab.screen)。
    var selectedTab: String = ""
    var sheet: Route?
    var fullScreen: Route?
    var showVoiceSheet = false

    struct Confirm: Identifiable {
        let id = UUID()
        var message: String
        var perform: @MainActor () async -> Void
    }
    var confirm: Confirm?
    var toast: String?
    private(set) var isMorphing = false

    let services: Services
    @ObservationIgnored private var syncTask: Task<Void, Never>?

    init(document: AppDocument, services: Services) {
        self.document = document
        self.services = services
        self.selectedTab = Self.initialTab(document.dsl)
    }

    /// initialScreen がタブの根ならそれ、違えば先頭タブ。
    static func initialTab(_ dsl: AppDSL) -> String {
        dsl.app.tabs.first { $0.screen == dsl.initialScreen }?.screen ?? dsl.app.tabs.first?.screen ?? ""
    }

    /// タブ用スタック名。
    static func tabStack(_ screen: String) -> String { "tab:" + screen }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    func bootstrap() async {
        Task { try? await services.transcriber.prepare() }
        await NotificationSync.sync(dsl.app.notifications)
        guard services.isLive else { return }
        if let remote = try? await services.appConfig.fetchDocument(appId: document.appId) {
            apply(remote, animated: false)
        }
        startSync()
    }

    func startSync() {
        syncTask?.cancel()
        let appId = document.appId
        let version = document.version
        syncTask = Task { [weak self] in
            guard let self else { return }
            for await doc in services.sync.updates(appId: appId, after: version) {
                if doc.version > document.version { apply(doc) }
            }
        }
    }

    /// 新しい DSL に差し替える。ナビ・キャッシュは初期化(画面構成が変わるため)。通知は DSL に同期。
    func apply(_ doc: AppDocument, animated: Bool = true) {
        let swap = {
            self.document = doc
            self.records = [:]
            self.paths = [:]
            self.sheet = nil
            self.fullScreen = nil
            self.showVoiceSheet = false
            self.selectedTab = Self.initialTab(doc.dsl)
        }
        if animated {
            withAnimation(.spring(duration: 0.7, bounce: 0.15), swap)
        } else {
            swap()
        }
        Task { await NotificationSync.sync(doc.dsl.app.notifications) }
    }

    /// 発話 → ai サービス → 最新 DSL 適用。
    func morph(transcript: String) async {
        guard !transcript.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return }
        isMorphing = true
        defer { isMorphing = false }
        do {
            let doc = try await services.ai.morph(transcript: transcript, appId: document.appId, currentVersion: document.version)
            apply(doc)
            startSync()
        } catch {
            toast = "生成に失敗しました: \(error.localizedDescription)"
        }
    }

    // MARK: - Records

    func records(for entity: String) -> [Record] { records[entity] ?? [] }

    func load(entities: Set<String>, force: Bool = false) async {
        for e in entities where force || records[e] == nil {
            await refresh(e)
        }
    }

    func refresh(_ entity: String) async {
        loading.insert(entity)
        defer { loading.remove(entity) }
        do {
            records[entity] = try await services.appConfig.list(appId: document.appId, entity: entity)
        } catch {
            toast = error.localizedDescription
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
            await attempt {
                if let id = form.recordId {
                    _ = try await services.appConfig.update(appId: document.appId, entity: entity, id: id, data: form.payload)
                } else {
                    _ = try await services.appConfig.create(appId: document.appId, entity: entity, data: form.payload)
                }
                await refresh(entity)
                if let screen = action.screen {
                    navigate(to: screen, presentation: action.presentation, recordId: nil, from: ctx.route)
                } else {
                    close(ctx.route)
                }
            }

        case .deleteRecord:
            guard let entity = action.entity ?? ctx.form?.entity.id,
                  let id = ctx.recordId ?? ctx.form?.recordId else { return }
            await attempt {
                try await services.appConfig.delete(appId: document.appId, entity: entity, id: id)
                await refresh(entity)
                if ctx.recordId == nil { close(ctx.route) } // フォーム上からの削除は画面を閉じる
            }

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
                try await NotificationSync.schedule(spec)
                toast = "通知を予約しました(\(spec.time ?? ""))"
            }

        case .cancelNotification:
            guard let id = action.notificationId else { return }
            NotificationSync.cancel(id: id)

        case .unknown:
            toast = "未対応のアクション: \(action.rawType)"
        }
    }

    private func attempt(_ body: () async throws -> Void) async {
        do { try await body() } catch { toast = error.localizedDescription }
    }
}
