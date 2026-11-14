// 審査用ビルド(APP_STORE_REVIEW)では外す(#614)。
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonServices

/// 自動の動きのルール 1 行(#347)。iOS(`AutomationsView`)と Mac(`MacAppSettingsSheet`・#979)で共有する。
struct AutomationRuleRow: View {
    let rule: AutomationRule
    var isPending = false
    /// オン / オフ(`rule.enabled` を返すサーバーだけ・作成者だけがこの画面を読める)
    var onToggle: ((Bool) -> Void)?
    /// 続けて失敗している時の「直しますか?」
    var onFix: (() -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            info
            if let enabled = rule.enabled, let onToggle {
                Toggle(isOn: Binding(get: { enabled }, set: { onToggle($0) })) {
                    Text(enabled ? LocalizedStringKey("オン") : "オフ").font(.subheadline)
                }
                .toggleStyle(.switch)
                .disabled(isPending)
                .help(Text("オフにすると、このルールは動きません。ルールは消えません。"))
                .accessibilityLabel(Text("「\(rule.description)」を動かす"))
                .accessibilityHint(Text("オフにすると、このルールは動きません。ルールは消えません。"))
                .accessibilityIdentifier("automations.toggle.\(rule.id)")
            }
            if rule.suggestsFix, let onFix {
                Button(action: onFix) {
                    Label("直しますか?", systemImage: "wrench.and.screwdriver")
                }
                .buttonStyle(.borderless)
                .font(.subheadline)
                .help(Text("直す依頼を入力欄に入れます"))
                .accessibilityHint(Text("直す依頼を入力欄に入れて、設定を閉じます"))
                .accessibilityIdentifier("automations.fix.\(rule.id)")
            }
        }
        .padding(.vertical, 2)
    }

    private var isOff: Bool { rule.enabled == false }

    private var info: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Label {
                    Text(verbatim: rule.description)
                } icon: {
                    Image(systemName: rule.trigger == .schedule ? "clock" : "bolt")
                }
                if isOff {
                    Text("オフ")
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, 6).padding(.vertical, 2)
                        .background(.quaternary, in: Capsule())
                }
            }
            .foregroundStyle(isOff ? .secondary : .primary)
            HStack(spacing: 6) {
                Image(systemName: Self.symbol(rule.result)).foregroundStyle(Self.color(rule.result))
                Text(Self.label(rule.result))
            }
            .font(.footnote)
            if rule.result == .failed, let reason = rule.failureReason {
                Text(Self.failureLabel(reason)).font(.caption).foregroundStyle(.secondary)
            }
            if rule.stopped != nil, rule.result != .stopped {
                Text("運営がこのルールを止めています。").font(.footnote).foregroundStyle(.orange)
            }
            if let at = rule.lastRunAt {
                Text("最後に動いた: \(at.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if rule.suggestsFix {
                Text("\(rule.consecutiveFailures) 回続けてうまくいっていません。")
                    .font(.footnote).foregroundStyle(.red)
            }
        }
        .accessibilityElement(children: .combine)
    }

    static func label(_ r: AutomationRule.Result) -> LocalizedStringKey {
        switch r {
        case .none: "まだ動いていません"
        case .ok: "動きました"
        case .notifyPaused: "動きました(通知は止めています)"
        case .limit: "今日の上限に達したため止めました"
        case .serviceLimit: "サービス全体の上限に達したため止めました"
        case .servicePaused: "サービス全体で一時停止しています"
        case .plan: "今のプランでは動きません"
        case .stopped: "運営が止めています"
        case .changed: "アプリが変わったため動きませんでした"
        case .unavailable: "アプリが使えない状態だったため動きませんでした"
        case .late: "遅れたため動かしませんでした"
        case .inactive: "しばらく開かれていないため休んでいます(開くと再開します)"
        case .failed: "何度か試しましたがうまくいきませんでした"
        case .disabled: "オフにしていたため動かしませんでした"
        case .other: "動きませんでした"
        }
    }

    /// 「何度か試しましたが…」の下に出す理由(#347)。サーバーは決まった語しか返さない。
    static func failureLabel(_ r: AutomationRule.FailureReason) -> LocalizedStringKey {
        switch r {
        case .notifyFailed: "通知を送れませんでした"
        case .recordUnavailable: "使うはずのデータが見つからないか、読めませんでした"
        case .mismatch: "ルールが今のアプリのデータの形と合っていません"
        case .writeRejected: "データを保存できませんでした"
        case .sourceUnavailable: "外部の情報を取りにいけませんでした"
        case .timeout: "時間内に終わりませんでした"
        case .internal: "サービス側の問題でした"
        }
    }

    static func symbol(_ r: AutomationRule.Result) -> String {
        switch r {
        case .none: "circle.dashed"
        case .ok: "checkmark.circle.fill"
        case .notifyPaused, .inactive, .late: "pause.circle.fill"
        case .disabled: "pause.circle"
        case .failed, .other, .unavailable, .changed: "exclamationmark.triangle.fill"
        case .limit, .serviceLimit, .servicePaused, .plan, .stopped: "hand.raised.fill"
        }
    }

    static func color(_ r: AutomationRule.Result) -> Color {
        switch r {
        case .none, .disabled: .secondary
        case .ok: .green
        case .notifyPaused, .inactive, .late: .orange
        default: .red
        }
    }
}
#endif
