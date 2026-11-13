#if DEBUG && METAMON_STG
import MetamonRenderer
import SwiftUI

/// 開発用メニュー: 端末の連絡先の一覧を読む口(#2218)を部品なしで試す。DEBUG の版だけ。
final class DebugDeviceContactsSection: DebugMenuSection {
    override class var order: Int { 260 }
    override class var title: String { "連絡先の一覧を読む" }
    override class var footer: String? {
        "許可を求めてから、名前・誕生日・グループの数を読みます(名前は先頭 5 人だけ出します)。iOS 18 で「一部の連絡先だけ」を選ぶと、許可した人だけが返ります。"
    }
    @MainActor override class func makeBody() -> AnyView { AnyView(DebugDeviceContactsRows()) }
}

private struct DebugDeviceContactsRows: View {
    @State private var result: String?

    var body: some View {
        Button {
            Task { await read() }
        } label: {
            DebugKnobLabel(title: "許可を求めて読む", effect: .immediate)
        }
        .accessibilityIdentifier("debugMenu.deviceContacts.read")
        if let result {
            Text(verbatim: result)
                .font(.footnote.monospaced())
                .textSelection(.enabled)
                .accessibilityIdentifier("debugMenu.deviceContacts.result")
        }
    }

    private func read() async {
        let source = DeviceContacts.source
        let access = await source.authorization(requesting: true)
        do {
            let people = try await source.contacts()
            let groups = try await source.groups()
            let head = people.prefix(5).map { "\($0.name) \($0.birthday?.isoString ?? "-") g\($0.groupIds.count)" }
            result = (["access: \(access.rawValue)", "people: \(people.count)", "groups: \(groups.count)"] + head).joined(separator: "\n")
        } catch {
            result = "access: \(access.rawValue)\nerror: \(error.localizedDescription)"
        }
    }
}
#endif
