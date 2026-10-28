import SwiftUI
import Observation
import MetamonDSL

/// Renderer がホスト(アプリ本体の `AppRuntime`)に要求する能力。Renderer は Services を知らず、この protocol 越しにだけ副作用を起こす。
/// `@Observable` な実体を `.environment(\.dslRuntime, runtime)` で注入する(プロパティアクセスは SwiftUI が追跡する)。
@MainActor
public protocol DSLRuntime: AnyObject, Observable {
    var dsl: AppDSL { get }
    /// スタック名(main / tab:xxx / sheet / full)→ push 済み Route
    var paths: [String: [Route]] { get }
    /// タブ表示時に選択中のタブ(= tab.screen)
    var selectedTab: String { get set }
    /// 読み込み中の entity id
    var loading: Set<String> { get }
    /// サーバーから取れずローカルにも無い entity id(#85)。空状態(まだありません)と区別して「読み込めませんでした」を描く
    var failed: Set<String> { get }

    func records(for entity: String) -> [Record]
    func load(entities: Set<String>, force: Bool) async
    func run(_ action: Action, _ ctx: ActionContext) async
    /// 既存レコードの一部フィールドだけを保存する(`list.item.toggle` のワンタップ チェック #108)。
    /// ローカル DB に即反映して同期キューが送る(フォームの saveRecord と同じ経路)。保存できなければ false(呼び出し側が表示を戻す)。
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool
    /// フォームを通さずに 1 件追加する(`locationTracker` の位置 1 点・#168)。経路は saveRecord と同じ(ローカル DB → 同期キュー)。
    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool
    /// back: スタックに積まれていれば pop、モーダルの最下層なら dismiss
    func close(_ route: Route)
    func pathBinding(_ stack: String) -> Binding<[Route]>
    /// `nearbyShare`(#36): 近くの端末を探すシートを開き、選んだ相手に `records`(entity の保存フィールドのみ)を送る。
    func shareNearby(entity: String, records: [[String: JSONValue]])
    /// `musicPlayer`(#412): 近くの端末を探すシートを開き、選んだ相手に曲のリクエストを送る。
    func requestSong(_ song: MusicSong)
    /// サインイン中の uid(共有レコードの作者判定・#249、`Field.hidden` の表示判定・#262)。nil なら判定しない(全部変更できる扱い)。
    var currentUserId: String? { get }
    /// 変更できないレコード(他人の共有レコード)を触った時。ホストが「作った人だけが変更できます」を出す。
    func notifyReadOnly(entity: String)
    /// 自分の印(いいね・保存 #255)を付け外しする。確定した状態を返し、付けられなければ nil(ホストが理由を出す)。
    func toggleMark(entity: String, recordId: String, mark: String) async -> MarkState?
    /// `timerDisplay.liveActivity`(#423): タイマーの開始・一時停止・リセット・終了をロック画面 / Dynamic Island に映す。
    func timerActivity(_ update: TimerActivityUpdate)
    /// aiButton generate(#417)のオンライン版。端末内 AI が使えない時だけ呼ぶ。同意・Pro の確認(画面を出す)もホストが持つ。
    func generateAIText(_ request: AITextRequest) async throws -> AITextResult
    /// オンライン版を持つホストか(プレビュー・テストは false = 端末内 AI だけ)。
    var supportsOnlineAIText: Bool { get }
}

public extension DSLRuntime {
    func load(entities: Set<String>) async { await load(entities: entities, force: false) }
    var failed: Set<String> { [] }
    /// 既定は保存しない(プレビュー・テスト用のランタイム)。
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool { false }
    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool { false }
    /// 既定は何もしない(プレビュー・テスト用ランタイム)。
    func shareNearby(entity: String, records: [[String: JSONValue]]) {}
    func requestSong(_ song: MusicSong) {}
    var currentUserId: String? { nil }
    func notifyReadOnly(entity: String) {}
    /// 既定は付けない(プレビュー・テスト用ランタイム)。
    func toggleMark(entity: String, recordId: String, mark: String) async -> MarkState? { nil }
    /// 既定は何もしない(プレビュー・テスト用ランタイム)。
    func timerActivity(_ update: TimerActivityUpdate) {}
    func generateAIText(_ request: AITextRequest) async throws -> AITextResult { throw AITextError.unsupported }
    var supportsOnlineAIText: Bool { false }

    /// 自分が更新できるレコードか(#249 / #252)。他人の共有レコードは entity.edit が members の時だけ。
    func canEdit(entity: String, record: Record) -> Bool {
        RecordPermission.canEdit(record, in: dsl.entity(entity), uid: currentUserId)
    }

    /// 自分が削除できるレコードか。共有レコードは作った人だけ。
    func canDelete(entity: String, record: Record) -> Bool {
        RecordPermission.canDelete(record, in: dsl.entity(entity), uid: currentUserId)
    }

    /// 部品が共通で使う空状態の判定: 読み込み中 / 取得失敗 / 本当に空。
    func emptyState(for entity: String) -> EmptyState {
        if loading.contains(entity) { return .loading }
        if failed.contains(entity) { return .unavailable }
        return .empty
    }
}

/// レコードが 0 件の時の理由。`EmptyPlaceholder` の文言・再試行ボタンを決める。
public enum EmptyState: Equatable, Sendable {
    case loading, unavailable, empty
}

private struct DSLRuntimeKey: EnvironmentKey {
    static let defaultValue: (any DSLRuntime)? = nil
}

/// 未注入時のフォールバック(何もしない・`AppDSL.fallback` を描く)。
/// `.environment(\.dslRuntime, x)` は WritableKeyPath 経由で getter を呼んでから setter を呼ぶため、getter で落とすと注入自体がクラッシュする。
@MainActor
@Observable
final class UnboundDSLRuntime: DSLRuntime {
    static let shared = UnboundDSLRuntime()

    var dsl: AppDSL = .fallback
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

public extension EnvironmentValues {
    /// Renderer の全 View が参照するランタイム。アプリ本体が `.environment(\.dslRuntime, runtime)` で注入する。
    var dslRuntime: any DSLRuntime {
        get { self[DSLRuntimeKey.self] ?? MainActor.assumeIsolated { UnboundDSLRuntime.shared } }
        set { self[DSLRuntimeKey.self] = newValue }
    }
}
