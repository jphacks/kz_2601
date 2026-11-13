import SwiftUI

/// 共有シートの拡張(MetamonShare)が App Group に置いた物を読む側(#2224)。
/// 拡張が `tailorit://share-inbox` で本体を開き、ここが受け取った一覧を出す。
/// どの生成アプリに入れるかの振り分けは #1775 の決定のあと。
@MainActor
@Observable
final class ShareInboxUI {
    static let shared = ShareInboxUI()

    var isShowing = false
    private(set) var items: [ShareInboxItem] = []
    private let inbox: ShareInbox?

    init(inbox: ShareInbox? = .shared) {
        self.inbox = inbox
    }

    /// 受け取り箱のリンクなら読み込んで開き true。それ以外の URL は false(触らない)。
    @discardableResult
    func receive(_ url: URL) -> Bool {
        guard ShareInbox.isOpenURL(url) else { return false }
        reload()
        isShowing = true
        return true
    }

    func reload() {
        items = inbox?.items() ?? []
    }

    func fileURL(of item: ShareInboxItem) -> URL? {
        inbox?.fileURL(of: item)
    }

    func remove(atOffsets offsets: IndexSet) {
        for item in offsets.map({ items[$0] }) { inbox?.remove(item) }
        reload()
    }

    func removeAll() {
        inbox?.removeAll()
        reload()
    }
}

/// WindowGroup の根に付ける: 共有シートから開かれたら受け取り箱を出す。
struct ShareInboxReceiver: ViewModifier {
    func body(content: Content) -> some View {
        content.onOpenURL { ShareInboxUI.shared.receive($0) }
    }
}

struct ShareInboxSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var ui = ShareInboxUI.shared

    var body: some View {
        NavigationStack {
            List {
                if ui.items.isEmpty {
                    Text(L10n.tr("受け取った物はありません"))
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(ui.items) { ShareInboxRow(item: $0) }
                        .onDelete { ui.remove(atOffsets: $0) }
                }
            }
            .navigationTitle(L10n.tr("受け取りました"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L10n.tr("閉じる")) { dismiss() }
                }
                ToolbarItem(placement: .primaryAction) {
                    Button(L10n.tr("全部消す"), role: .destructive) { ui.removeAll() }
                        .disabled(ui.items.isEmpty)
                }
            }
        }
        .accessibilityIdentifier("shareInbox.sheet")
    }
}

private struct ShareInboxRow: View {
    let item: ShareInboxItem

    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .lineLimit(2)
                Text(item.receivedAt.formatted(date: .abbreviated, time: .shortened))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        } icon: {
            Image(systemName: symbol)
        }
    }

    private var title: String {
        item.text ?? item.fileName ?? L10n.tr("受け取った物")
    }

    private var symbol: String {
        switch item.kind {
        case .text: "text.alignleft"
        case .url: "link"
        case .image: "photo"
        case .pdf: "doc.richtext"
        }
    }
}

/// RootView に 1 行で付ける: 受け取り箱のシート。
struct ShareInboxHost: ViewModifier {
    @State private var ui = ShareInboxUI.shared

    func body(content: Content) -> some View {
        content.sheet(isPresented: Binding(get: { ui.isShowing }, set: { ui.isShowing = $0 })) {
            ShareInboxSheet()
        }
    }
}
