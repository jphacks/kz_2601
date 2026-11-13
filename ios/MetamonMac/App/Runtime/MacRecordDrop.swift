import SwiftUI
import UniformTypeIdentifiers
import MetamonDSL
import MetamonRenderer

/// 記録の行をサイドバーの別のアプリへドラッグする(#1891 の Mac・#2252)。落とされたアプリの新しいフォームを開き、
/// `RecordClipboard` の対応(⌘V の貼り付けと同じ)で値を入れる。保存はしない。
enum MacRecordDrop {
    /// 落とせる型(行のドラッグが載せる記録)
    static let type = UTType(exportedAs: RecordClipboard.typeIdentifier)

    /// 値を入れる先の候補(entity ごとに 1 つのフォーム画面)
    struct Target: Equatable, Identifiable {
        var entity: String
        var name: String
        var screen: String
        /// 入る項目の数
        var matched: Int
        var id: String { entity }
    }

    /// 落とし先の entity を決める: そのアプリのフォーム画面(entity のある画面)のうち、合う項目が 1 つ以上ある entity。
    /// 並びは画面の順。同じ entity のフォームが複数あれば先の画面を使う。0 件なら合うフォームが無い・2 件以上なら選んでもらう
    static func targets(_ payload: RecordClipboard.Payload, dsl: AppDSL, appId: String, isAdmin: Bool = false) -> [Target] {
        var seen: Set<String> = []
        var out: [Target] = []
        for screen in dsl.screens {
            guard let entityId = screen.entity, !seen.contains(entityId), let entity = dsl.entity(entityId) else { continue }
            seen.insert(entityId)
            let result = RecordClipboard.paste(payload, into: entity, appId: appId, isAdmin: isAdmin)
            guard !result.matched.isEmpty else { continue }
            out.append(Target(entity: entityId, name: entity.name.isEmpty ? screen.title : entity.name,
                              screen: screen.id, matched: result.matched.count))
        }
        return out
    }

    /// 落とされた物から記録を読む。記録が無い・壊れている時は nil
    static func load(_ providers: [NSItemProvider]) async -> RecordClipboard.Payload? {
        guard let provider = providers.first(where: { $0.hasItemConformingToTypeIdentifier(RecordClipboard.typeIdentifier) }) else {
            return nil
        }
        let data: Data? = await withCheckedContinuation { done in
            _ = provider.loadDataRepresentation(forTypeIdentifier: RecordClipboard.typeIdentifier) { data, _ in
                done.resume(returning: data)
            }
        }
        return data.flatMap(RecordClipboard.decode)
    }
}

/// 落とされた記録を、そのアプリの中央(`AppCanvas`)ができるまで預かる(全ウインドウで 1 つ)。古いものは捨てる
@MainActor
@Observable
final class MacRecordDropInbox {
    static let shared = MacRecordDropInbox()
    /// これより古い預かりは捨てる(アプリの読み込みを待つ分)
    nonisolated static let ttl: TimeInterval = 60

    /// 預かった回数(`AppCanvas` が見張る)
    private(set) var serial = 0
    @ObservationIgnored private var pending: (appId: String, payload: RecordClipboard.Payload, at: Date)?

    func put(_ payload: RecordClipboard.Payload, appId: String, now: Date = .now) {
        pending = (appId, payload, now)
        serial &+= 1
    }

    /// このアプリ宛ての記録を受け取る(1 回だけ)
    func take(appId: String, now: Date = .now) -> RecordClipboard.Payload? {
        guard let p = pending, p.appId == appId else { return nil }
        pending = nil
        return now.timeIntervalSince(p.at) < Self.ttl ? p.payload : nil
    }
}

extension MacAppRuntime {
    /// 落とされた記録で `target` のフォームを新しく開き、合う項目に入れる(保存はしない)。結果の一言は ⌘V と同じ
    func openDroppedRecord(_ payload: RecordClipboard.Payload, into target: MacRecordDrop.Target) async {
        let from = NavigationResolver.top(navigationState, dsl: dsl)
        navigate(to: target.screen, presentation: .sheet, recordId: nil, from: from)
        let route = NavigationResolver.top(navigationState, dsl: dsl)
        // フォームが描かれて控え(`openForms`)に載るのを待つ
        for _ in 0..<40 {
            if let form = openForms.form(at: route) {
                RecordPasteboard.apply(payload, into: form, rt: self)
                return
            }
            try? await Task.sleep(for: .milliseconds(50))
        }
    }
}
