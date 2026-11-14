#if !APP_STORE_REVIEW
import SwiftUI
import MetamonRenderer
import MetamonServices

/// 通報の理由・ブロックの確認・結果・18 歳以上の確認(#963)。右クリックのメニュー(Renderer の `ModerationMenuItems`)から開く。
/// `active` は確認ダイアログと同じ(シートが開いている間はシートの中だけに出す)。
struct MacSocialDialogs: ViewModifier {
    let runtime: MacAppRuntime
    let active: Bool

    private var social: MacSocial { runtime.social }

    func body(content: Content) -> some View {
        content
            .confirmationDialog(Text("通報の理由"), isPresented: Binding(
                get: { active && social.reporting != nil },
                set: { if !$0 { social.reporting = nil } }
            ), titleVisibility: .visible, presenting: social.reporting) { req in
                ForEach(ReportReason.allCases) { reason in
                    Button(reason.macLabel) { Task { await runtime.submitReport(req, reason: reason) } }
                }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("運営が内容を確認します。通報したことは相手に知らされません。")
            }
            .alert(Text("「\(social.blocking?.authorName ?? "")」さんをブロックしますか?"), isPresented: Binding(
                get: { active && social.blocking != nil },
                set: { if !$0 { social.blocking = nil } }
            ), presenting: social.blocking) { req in
                Button("ブロックする", role: .destructive) { Task { await runtime.confirmBlock(req) } }
                Button("キャンセル", role: .cancel) {}
            } message: { _ in
                Text("この人の投稿と公開したアプリが、あなたに表示されなくなります。相手には知らされません。設定からいつでも解除できます。")
            }
            .alert(Text(social.notice?.title ?? ""), isPresented: Binding(
                get: { active && social.notice != nil },
                set: { if !$0 { social.notice = nil } }
            ), presenting: social.notice) { _ in
                Button("OK", role: .cancel) {}
            } message: { notice in
                Text(notice.message)
            }
            .sheet(item: Binding(
                get: { active ? social.askingAge : nil },
                set: { if $0 == nil, social.askingAge != nil { social.askingAge = nil; social.declinedAge = true } }
            )) { pending in
                MacAdultConfirmSheet(
                    confirm: { Task { await runtime.confirmAdult(pending) } },
                    decline: { runtime.declineAdult() })
            }
            .task(id: runtime.currentUserId) {
                await runtime.loadBlockedUsers()
                if MacAppRuntime.usesMembers(runtime.dsl) { await runtime.reloadAppMembers() }
            }
    }
}

/// 「18 歳以上ですか?」(#507)。iOS の `AdultConfirmSheet` と同じ文言
struct MacAdultConfirmSheet: View {
    let confirm: () -> Void
    let decline: () -> Void

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "person.badge.shield.checkmark")
                .font(.system(size: 40))
                .foregroundStyle(.tint)
                .accessibilityHidden(true)
            Text("18 歳以上ですか?")
                .font(.title2.weight(.bold))
            Text("仲間探しでは、知らない人と「つながりたい」を送り合い、お互いに承認すると連絡先が見えるようになります。18 歳以上の方だけが使えます。")
                .multilineTextAlignment(.center)
            Text("年齢や生年月日は保存しません。18 歳以上と答えたことだけを、あなたのアカウントに記録します。")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
            HStack {
                Button("18 歳未満です", action: decline)
                    .accessibilityIdentifier("matching.adult.decline")
                Button("18 歳以上です", action: confirm)
                    .keyboardShortcut(.defaultAction)
                    .accessibilityIdentifier("matching.adult.confirm")
            }
            .controlSize(.large)
            .padding(.top, 6)
        }
        .padding(24)
        .frame(width: 420)
    }
}
#endif
