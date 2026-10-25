import Foundation
import Observation

/// 変身一覧からのアプリ削除(#180)。切り替え先の決定と、楽観的 UI(先に隠す → 失敗で戻す)の状態だけを持つ。
public enum AppDeletion {
    /// 使用中のアプリを消した時の行き先。
    public enum Fallback: Equatable, Sendable {
        /// 残りのうち最新のアプリ
        case app(String)
        /// 残りが無ければ初期(音声)画面
        case home
    }

    /// `id` を消した後の切り替え先。使用中のアプリでなければ nil(切り替えない)。
    /// 候補は一覧と同じ基準(archived・未生成を除く)で、更新日時の新しい順の先頭。
    public static func fallback(deleting id: String, current: String?, apps: [AppSummary]) -> Fallback? {
        guard id == current else { return nil }
        let rest = AppSummary.visible(apps.filter { $0.id != id }, current: nil)
        return rest.first.map { .app($0.id) } ?? .home
    }
}

/// 削除中のアプリを一覧から先に隠す(楽観的 UI)。サーバー削除が失敗したら隠すのをやめる = 一覧に戻る。
/// 一覧(`apps`)そのものは書き換えないので、削除中に一覧を取り直しても消したカードが復活しない。
@MainActor
@Observable
public final class OptimisticAppDeletion {
    public private(set) var hidden: Set<String> = []

    public init() {}

    public func filter(_ apps: [AppSummary]) -> [AppSummary] {
        hidden.isEmpty ? apps : apps.filter { !hidden.contains($0.id) }
    }

    /// `id` を隠してから `delete` を待つ。成功時は `onSuccess`(一覧から実際に外す等)を **隠したまま** 呼んでから隠すのをやめる
    /// (カードが一瞬戻らない)。失敗時は隠すのをやめてエラーをそのまま投げる。
    public func run(_ id: String, delete: () async throws -> Void, onSuccess: () -> Void) async throws {
        hidden.insert(id)
        defer { hidden.remove(id) }
        try await delete()
        onSuccess()
    }
}
