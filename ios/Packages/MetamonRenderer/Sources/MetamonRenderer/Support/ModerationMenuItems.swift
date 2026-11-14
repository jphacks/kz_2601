import SwiftUI
import MetamonDSL

/// 他人の共有レコードの長押しメニューに足す「通報する」「投稿者をブロック」(#321・審査 1.2)。
/// 出すかどうかはホストが決める(`ModerationHost.canReport`)。シェアの無い版・自分の投稿・private の entity では何も出さない。
struct ModerationMenuItems: View {
    @Environment(\.dslRuntime) private var rt
    let entity: String
    let record: Record

    var body: some View {
        ForEach(Self.actions(rt, entity: entity, record: record), id: \.label) { a in
            Button(a.label, systemImage: a.systemImage, role: a.role, action: a.perform)
        }
    }

    /// 通報・ブロックの操作。長押しメニューと VoiceOver の行の操作(`ModerationAccessibilityActions`)で同じものを出す。
    struct Action {
        let label: String
        let systemImage: String
        let role: ButtonRole?
        let perform: () -> Void
    }

    static func actions(_ rt: any DSLRuntime, entity: String, record: Record) -> [Action] {
        guard let host = rt.moderation, host.canReport(entity: entity, record: record) else { return [] }
        return [
            Action(label: L("通報する"), systemImage: "exclamationmark.bubble", role: nil) { host.reportRecord(entity: entity, record: record) },
            Action(label: L("投稿者をブロック"), systemImage: "hand.raised", role: .destructive) { host.blockAuthor(entity: entity, record: record) },
        ]
    }
}

/// `.accessibilityActions` の中に置く「通報する」「投稿者をブロック」。VoiceOver では長押しメニューが選べないので、行の操作からも選べるようにする。
struct ModerationAccessibilityActions: View {
    let rt: any DSLRuntime
    let entity: String
    let record: Record

    var body: some View {
        ForEach(ModerationMenuItems.actions(rt, entity: entity, record: record), id: \.label) { a in
            Button(a.label, action: a.perform)
        }
    }
}
