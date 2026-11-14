// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonDSL

/// `nearbyShare`(#36): 近くの Tailor It 端末へレコードを送るボタン。通信と相手選びはホスト(`NearbyHost.shareNearby`)。
/// 送る範囲: フォーム画面(screen.entity == entity)なら入力中の 1 件、それ以外はその entity の新しい順 最大 50 件。
/// mode:passby(すれ違い通信・#425)は `NearbyPassbyLeaf`(Mac では使えない・#964)。
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

    /// 描き分け。すれ違い通信は NearbyInteraction(UWB)と持ち歩く前提の機能なので、Mac では「この Mac では使えません」の枠(#964・mac-025)。
    /// 送る方(MultipeerConnectivity)は Mac でも iPhone・iPad と送り合える。
    enum Kind: Equatable {
        case send
        case passby(NearbyPassby.Config)
        case unavailable(feature: String)
    }

    static func kind(of c: Component) -> Kind {
        guard let config = NearbyPassby.Config(c) else { return .send }
        #if os(macOS)
        return .unavailable(feature: config.label) // 部品の label(無ければ「すれ違い通信」)
        #else
        return .passby(config)
        #endif
    }

    var body: some View {
        switch Self.kind(of: c) {
        case .send:
            sendButton
        case .passby(let config):
            NearbyPassbyLeaf(c: c, config: config)
        case .unavailable(let feature):
            #if os(macOS)
            MacUnavailableView(feature: feature).renderCard()
            #else
            EmptyView()
            #endif
        }
    }

    @ViewBuilder private var sendButton: some View {
        let rows = records
        Button {
            runtime.nearby?.shareNearby(entity: entity, records: rows)
        } label: {
            Label(c.label ?? L("近くの人に送る"), systemImage: "antenna.radiowaves.left.and.right")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(RenderButtonStyle(kind: .secondary))
        .disabled(rows.isEmpty)
        .task(id: entity) {
            if ctx.form?.entity.id != entity { await runtime.load(entities: [entity]) }
        }
        .accessibilityHint(rows.isEmpty ? L("送る記録がありません") : L("近くの端末を探して \(rows.count) 件を送ります"))
    }
}
#endif
