import SwiftUI
import Observation

/// NavigationStack の path 要素。画面 id + params。
struct Route: Hashable, Identifiable {
    var screen: String
    var params: [String: JSONValue] = [:]
    var id: String { "\(screen)#\(params.hashValue)" }
}

/// アプリ全体の単一ランタイム。DSL ドキュメント・動的 state・ナビゲーション・アクション実行を担う。
@MainActor
@Observable
final class AppRuntime {
    private(set) var document: AppDocument
    var state: JSONValue
    var user: JSONValue = .null

    var selectedTab: String = "main"
    var paths: [String: [Route]] = [:]
    var sheet: Route?
    var fullScreen: Route?
    var showVoiceSheet = false

    var toast: String?
    private(set) var isMorphing = false

    let services: Services
    @ObservationIgnored private var syncTask: Task<Void, Never>?

    init(document: AppDocument, services: Services) {
        self.document = document
        self.state = document.state ?? .object([:])
        self.services = services
        self.selectedTab = document.navigation.tabs.first?.id ?? "main"
    }

    // MARK: - Lifecycle

    /// 起動時: live ならサーバーの最新 DSL を取得しホットリロード開始。文字起こしモデルも温めておく。
    func bootstrap() async {
        Task { try? await services.transcriber.prepare() }
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

    /// 新しい DSL に差し替える。state/ナビは初期化(画面構成が変わるため)。
    func apply(_ doc: AppDocument, animated: Bool = true) {
        let swap = {
            self.document = doc
            self.state = doc.state ?? .object([:])
            self.paths = [:]
            self.sheet = nil
            self.fullScreen = nil
            self.showVoiceSheet = false
            self.selectedTab = doc.navigation.tabs.first?.id ?? "main"
        }
        if animated {
            withAnimation(.spring(duration: 0.7, bounce: 0.15), swap)
        } else {
            swap()
        }
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

    // MARK: - Expressions / Bindings

    private func scope(_ ctx: RenderContext) -> Interpolator.Scope {
        .init(state: state, user: user, ctx: ctx)
    }

    func resolve(_ template: String?, _ ctx: RenderContext) -> JSONValue {
        guard let template else { return .null }
        return Interpolator.resolve(template, in: scope(ctx))
    }

    func resolve(_ value: JSONValue, _ ctx: RenderContext) -> JSONValue {
        Interpolator.resolve(value, in: scope(ctx))
    }

    func string(_ template: String?, _ ctx: RenderContext) -> String {
        resolve(template, ctx).displayString
    }

    func isVisible(_ node: Node, _ ctx: RenderContext) -> Bool {
        guard let v = node.visible else { return true }
        if case .string(let s) = v { return resolve(s, ctx).truthy }
        return v.truthy
    }

    func binding(_ path: String) -> Binding<JSONValue> {
        let p = Interpolator.statePath(path)
        return Binding(
            get: { self.state[path: p] ?? .null },
            set: { self.state.set(path: p, to: $0) }
        )
    }

    func stringBinding(_ path: String) -> Binding<String> {
        let b = binding(path)
        return Binding(get: { b.wrappedValue.displayString }, set: { b.wrappedValue = .string($0) })
    }

    func boolBinding(_ path: String) -> Binding<Bool> {
        let b = binding(path)
        return Binding(get: { b.wrappedValue.truthy }, set: { b.wrappedValue = .bool($0) })
    }

    // MARK: - Navigation

    private var currentTab: String {
        document.navigation.type == .tab ? selectedTab : "main"
    }

    func pathBinding(_ tab: String) -> Binding<[Route]> {
        Binding(get: { self.paths[tab] ?? [] }, set: { self.paths[tab] = $0 })
    }

    // MARK: - Actions

    func run(_ action: Action, _ ctx: RenderContext) async {
        let appId = document.appId
        switch action {
        case .navigate(let screen, let params):
            paths[currentTab, default: []].append(Route(screen: screen, params: resolveParams(params, ctx)))

        case .back:
            if !(paths[currentTab] ?? []).isEmpty { paths[currentTab]?.removeLast() }

        case .present(let screen, let params, let style):
            let route = Route(screen: screen, params: resolveParams(params, ctx))
            if style == .sheet { sheet = route } else { fullScreen = route }

        case .dismiss:
            sheet = nil
            fullScreen = nil

        case .setState(let path, let value):
            state.set(path: Interpolator.statePath(path), to: resolve(value, ctx))

        case .entityList(let entity, let into, let query):
            await attempt {
                let rows = try await services.appConfig.list(appId: appId, entity: entity, query: query.map { resolve($0, ctx) })
                state.set(path: Interpolator.statePath(into), to: .array(rows))
            }

        case .entityCreate(let entity, let data):
            await attempt { _ = try await services.appConfig.create(appId: appId, entity: entity, data: resolve(data, ctx)) }

        case .entityUpdate(let entity, let id, let data):
            await attempt {
                _ = try await services.appConfig.update(appId: appId, entity: entity, id: string(id, ctx), data: resolve(data, ctx))
            }

        case .entityDelete(let entity, let id):
            await attempt { try await services.appConfig.delete(appId: appId, entity: entity, id: string(id, ctx)) }

        case .native(let feature, let args, let into):
            await attempt {
                let result = try await services.native.perform(feature, args: resolve(args, ctx))
                if let into { state.set(path: Interpolator.statePath(into), to: result) }
            }

        case .openURL(let url):
            await attempt { _ = try await services.native.perform("openURL", args: ["url": .string(string(url, ctx))]) }

        case .aiPrompt(let text):
            await morph(transcript: string(text, ctx))

        case .sequence(let actions):
            for a in actions { await run(a, ctx) }

        case .unknown(let type):
            toast = "未対応のアクション: \(type)"
        }
    }

    private func resolveParams(_ params: [String: JSONValue], _ ctx: RenderContext) -> [String: JSONValue] {
        params.mapValues { resolve($0, ctx) }
    }

    private func attempt(_ body: () async throws -> Void) async {
        do { try await body() } catch { toast = error.localizedDescription }
    }
}
