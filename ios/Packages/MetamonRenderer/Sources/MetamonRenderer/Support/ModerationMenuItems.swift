import SwiftUI
import MetamonDSL

/// 他人の共有レコードの長押しメニューに足す「通報する」「投稿者をブロック」(#321・審査 1.2)。
/// 出すかどうかはホストが決める(`DSLRuntime.canReport`)。シェアの無い版・自分の投稿・private の entity では何も出さない。
struct ModerationMenuItems: View {
    @Environment(\.dslRuntime) private var rt
    let entity: String
    let record: Record

    var body: some View {
        if rt.canReport(entity: entity, record: record) {
            Button(L("通報する"), systemImage: "exclamationmark.bubble") { rt.reportRecord(entity: entity, record: record) }
            Button(L("投稿者をブロック"), systemImage: "hand.raised", role: .destructive) { rt.blockAuthor(entity: entity, record: record) }
        }
    }
}
