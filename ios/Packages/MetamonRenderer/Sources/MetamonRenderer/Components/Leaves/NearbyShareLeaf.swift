// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `nearbyShare`(#36): 近くの Tailor It 端末へレコードを送るボタン。通信と相手選びはホスト(`DSLRuntime.shareNearby`)。
/// 送る範囲: フォーム画面(screen.entity == entity)なら入力中の 1 件、それ以外はその entity の新しい順 最大 50 件。
/// mode:passby(すれ違い通信・#425)は `NearbyPassbyLeaf`。
struct NearbyShareLeaf: View {
    @Environment(\.dslRuntime) private var runtime
    let c: Component
    let ctx: RenderContext

    private var entity: String { c.entity ?? "" }

    private var records: [[String: JSONValue]] {
        if let form = ctx.form, form.entity.id == entity {
            let draft = form.payload
            return draft.isEmpty ? [] : [draft]
        }
        return NearbyShare.recordsToSend(from: runtime.records(for: entity))
    }

    var body: some View {
        if let config = NearbyPassby.Config(c) {
            NearbyPassbyLeaf(c: c, config: config)
        } else {
            sendButton
        }
    }

    @ViewBuilder private var sendButton: some View {
        let rows = records
        Button {
            runtime.shareNearby(entity: entity, records: rows)
        } label: {
            Label(c.label ?? "近くの人に送る", systemImage: "antenna.radiowaves.left.and.right")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: .secondary))
        .disabled(rows.isEmpty)
        .task(id: entity) {
            if ctx.form?.entity.id != entity { await runtime.load(entities: [entity]) }
        }
        .accessibilityHint(rows.isEmpty ? "送る記録がありません" : "近くの端末を探して \(rows.count) 件を送ります")
    }
}
#endif
