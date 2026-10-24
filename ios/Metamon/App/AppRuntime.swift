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
    /// エラー等の短い通知(下部カプセル)。生成の summary はこちらではなく `summaryBanner`。
    var toast: String?

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

    let services: Services
    @ObservationIgnored private var syncTask: Task<Void, Never>?
    @ObservationIgnored private var morphTask: Task<MorphResult, Error>?

    init(document: AppDocument, services: Services) {
        self.document = document
        self.services = services
        self.selectedTab = document.dsl.initialTab
    }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    func bootstrap() async {
        Task { try? await services.transcriber.prepare() }
        await services.notifications.sync(dsl.app.notifications)
        if services.isLive {
            if let remote = try? await services.appConfig.fetchDocument(appId: document.appId) {
                apply(remote, animated: false)
            }
            startSync()
        }
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_AUTO_TRANSCRIPT="家計簿アプリが欲しい"` で起動直後に 1 回だけ生成を投げる
        if let text = ProcessInfo.processInfo.environment["METAMON_AUTO_TRANSCRIPT"], !text.isEmpty {
            await morph(transcript: text)
        }
        #endif
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
            self.selectedTab = doc.dsl.initialTab
            self.morphJob = nil
        }
        if animated {
            withAnimation(.spring(duration: 0.65, bounce: 0.12), swap)
        } else {
            swap()
        }
        Task { await services.notifications.sync(doc.dsl.app.notifications) }
    }

    /// 発話 → ai サービス → 最新 DSL 適用。
    /// 生成中は `morphJob` が立ち、RootView が create(全画面「変身中」)/ edit(薄いインジケータ)を出す。
    /// 完了時は新ツリーの挿入と生成中オーバーレイの消去を**同じアニメーション**で行い、クロスフェードにする。
    func morph(transcript: String) async {
        let text = transcript.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, morphJob == nil else { return }
        withAnimation(.easeInOut(duration: 0.35)) {
            morphJob = MorphJob(kind: dsl.isVoiceOnly ? .create : .edit, transcript: text, startedAt: .now)
        }
        let task = Task { [services, appId = document.appId, version = document.version] in
            try await services.ai.morph(transcript: text, appId: appId, currentVersion: version)
        }
        morphTask = task
        defer { morphTask = nil }
        do {
            let result = try await task.value
            apply(result.document) // withAnimation の中で morphJob = nil も行う
            startSync()
            if let summary = result.summary?.trimmingCharacters(in: .whitespacesAndNewlines), !summary.isEmpty {
                summaryBanner = SummaryBanner(text: summary, highlight: PositionHint.anchor(in: summary))
            }
            print("[Metamon] morph ok: version=\(result.document.version) mode=\(result.mode ?? "-") screens=\(result.document.dsl.screens.map(\.id)) summary=\(result.summary ?? "")")
        } catch is CancellationError {
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "生成をキャンセルしました"
        } catch {
            print("[Metamon] morph failed: \(error)")
            withAnimation(.easeInOut(duration: 0.3)) { morphJob = nil }
            toast = "生成に失敗しました: \(error.localizedDescription)"
        }
    }

    /// 生成の中断(30 秒超の長引きで UI から呼ぶ)。URLSession は Task キャンセルで切れる。
    func cancelMorph() {
        morphTask?.cancel()
    }

    /// アプリを初期状態(音声画面のみ)に戻す。appId を作り直すので ai/appconfig 側からも「新規端末」に見える(旧データは削除しない)。
    func resetToInitial() {
        syncTask?.cancel()
        var fresh = (try? DSLExamples.document(DSLExamples.initial, version: 0)) ?? .fallback
        fresh.appId = DeviceAppID.rotate()
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

    private func attempt(_ body: () async throws -> Void) async {
        do { try await body() } catch { toast = error.localizedDescription }
    }
}
