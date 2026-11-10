import Foundation
import Observation
import MetamonDSL

/// 会話で作った・直したアプリの DSL と、その版の「元に戻す / やり直す」(#967・#968)。
/// サーバー(appconfig の versions)が無い間(#930 の前)は端末の履歴(`LocalVersionHistory`)だけで戻す。
/// iOS もサーバーに届かない時は端末の履歴で戻す(`AppRuntime.undo`)。ウインドウを閉じれば消える(iOS の METAMON_MOCK と同じ)。
/// 版の nil は「同梱の例のまま(会話で直す前)」。同梱の例を直して戻すと、元の例を表示し直す。
@MainActor
@Observable
final class MacAppLibrary {
    private(set) var generated: [String: AppDocument] = [:]
    /// 会話で作ったアプリの id(新しい順)
    private(set) var createdIds: [String] = []
    private var histories: [String: LocalVersionHistory<AppDocument?>] = [:]

    /// 会話の結果を当てる。作った直後の版には戻り先が無い(iOS も作った直後は「元に戻す」を出さない)
    func apply(_ document: AppDocument, created: Bool) {
        let id = document.appId
        if !(created && generated[id] == nil) {
            histories[id, default: .init()].record(previous: generated[id])
        }
        generated[id] = document
        if created, !createdIds.contains(id) { createdIds.insert(id, at: 0) }
    }

    func canUndo(_ appId: String) -> Bool { histories[appId]?.canUndo ?? false }
    func canRedo(_ appId: String) -> Bool { histories[appId]?.canRedo ?? false }

    /// 「元に戻す」。戻せたら true
    @discardableResult
    func undo(_ appId: String) -> Bool {
        guard var history = histories[appId], let previous = history.undo(from: generated[appId]) else { return false }
        histories[appId] = history
        generated[appId] = previous
        return true
    }

    /// 「やり直す」。やり直せたら true
    @discardableResult
    func redo(_ appId: String) -> Bool {
        guard var history = histories[appId], let next = history.redo(from: generated[appId]) else { return false }
        histories[appId] = history
        generated[appId] = next
        return true
    }
}
