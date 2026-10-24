import SwiftUI
import MetamonServices

/// エラーの **状態カード**(#60)。トーストではなく、音声画面 / 生成後の画面上部に出す「アイコン・1 文・主ボタン・副ボタン」。
/// どのエラーでも「次に何をすればいいか」が画面に残り、押せばその場で復帰する。文言は `ErrorCopy` に集約。
struct ErrorPresentation: Equatable, Identifiable {
    enum Kind: Equatable {
        /// 圏外で送れず保留中(復帰で自動送信)
        case offlineQueued
        /// 生成中に通信が切れた / 圏外で失敗した
        case offlineInterrupted
        /// 429 / 5xx(混雑)
        case busy(retryAfter: TimeInterval?)
        /// タイムアウト(45s / 504)
        case timeout
        /// 401(再取得しても失効)
        case sessionExpired
        /// 4xx(入力不備など)・その他
        case failed(message: String)
    }
    enum Primary: Equatable { case retry, sendNow, signIn }
    enum Secondary: Equatable { case discard, dismiss }

    let id = UUID()
    var kind: Kind
    /// 再送に使う発話
    var transcript: String

    static func == (a: ErrorPresentation, b: ErrorPresentation) -> Bool { a.kind == b.kind && a.transcript == b.transcript }

    /// `FailureKind`(Services の分類)→ 表示。`conflict` はカードにせず呼び出し側がバナー + 最新取り直しで処理する。
    static func from(_ failure: FailureKind, transcript: String) -> ErrorPresentation? {
        switch failure {
        case .offline: return ErrorPresentation(kind: .offlineInterrupted, transcript: transcript)
        case .timeout: return ErrorPresentation(kind: .timeout, transcript: transcript)
        case .busy(let after): return ErrorPresentation(kind: .busy(retryAfter: after), transcript: transcript)
        case .sessionExpired: return ErrorPresentation(kind: .sessionExpired, transcript: transcript)
        case .rejected(let m), .other(let m): return ErrorPresentation(kind: .failed(message: m), transcript: transcript)
        case .conflict, .cancelled: return nil
        }
    }

    var icon: String {
        switch kind {
        case .offlineQueued: "wifi.slash"
        case .offlineInterrupted: "wifi.exclamationmark"
        case .busy: "hourglass"
        case .timeout: "clock.badge.exclamationmark"
        case .sessionExpired: "person.crop.circle.badge.exclamationmark"
        case .failed: "exclamationmark.triangle"
        }
    }

    var title: String {
        switch kind {
        case .offlineQueued: ErrorCopy.offlineQueuedTitle
        case .offlineInterrupted: ErrorCopy.offlineInterruptedTitle
        case .busy: ErrorCopy.busyTitle
        case .timeout: ErrorCopy.timeoutTitle
        case .sessionExpired: ErrorCopy.sessionExpiredTitle
        case .failed: ErrorCopy.failedTitle
        }
    }

    var body: String {
        switch kind {
        case .offlineQueued: ErrorCopy.offlineQueuedBody
        case .offlineInterrupted: ErrorCopy.offlineInterruptedBody
        case .busy(let after): after.map { $0 > 1 ? ErrorCopy.busyBody(retryAfter: $0) : ErrorCopy.busyBody } ?? ErrorCopy.busyBody
        case .timeout: ErrorCopy.timeoutBody
        case .sessionExpired: ErrorCopy.sessionExpiredBody
        case .failed(let message): message
        }
    }

    var primary: Primary? {
        switch kind {
        case .offlineQueued: .sendNow
        case .offlineInterrupted, .busy, .timeout, .failed: .retry
        case .sessionExpired: .signIn
        }
    }

    var secondary: Secondary {
        switch kind {
        case .offlineQueued: .discard
        default: .dismiss
        }
    }

    var isWarning: Bool {
        if case .failed = kind { return true }
        if case .sessionExpired = kind { return true }
        return false
    }
}

/// 状態カードの見た目。`Theme` のカード(角丸・影)に合わせ、主ボタンはブランド色、副ボタンは文字だけ。
struct ErrorCard: View {
    @Environment(AppRuntime.self) private var rt
    let presentation: ErrorPresentation
    /// 生成後の画面上部(コンパクト)か、音声画面の中(通常)か
    var compact = false

    var body: some View {
        HStack(alignment: .top, spacing: Theme.Spacing.sm + 2) {
            Image(systemName: presentation.icon)
                .font(.title3.weight(.semibold))
                .foregroundStyle(presentation.isWarning ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(Theme.brandGradient))
                .frame(width: 28)
                .padding(.top, 1)
            VStack(alignment: .leading, spacing: Theme.Spacing.xs + 2) {
                Text(presentation.title)
                    .font(.subheadline.weight(.semibold))
                Text(presentation.body)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if !presentation.transcript.isEmpty, case .offlineQueued = presentation.kind {
                    Text("「\(presentation.transcript)」")
                        .font(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
                HStack(spacing: Theme.Spacing.md) {
                    if let primary = presentation.primary {
                        Button { rt.handle(primary, of: presentation) } label: {
                            Text(label(primary))
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(.white)
                                .padding(.horizontal, Theme.Spacing.md)
                                .padding(.vertical, Theme.Spacing.sm)
                                .background(Theme.brandGradient, in: Capsule())
                        }
                        .buttonStyle(.plain)
                        .accessibilityIdentifier("error.primary")
                    }
                    Button { rt.handle(presentation.secondary, of: presentation) } label: {
                        Text(label(presentation.secondary))
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.secondary)
                            .padding(.vertical, Theme.Spacing.sm)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("error.secondary")
                }
                .padding(.top, Theme.Spacing.xs)
            }
            Spacer(minLength: 0)
        }
        .metamonCard(padding: compact ? Theme.Spacing.md : Theme.Spacing.md + Theme.Spacing.xs)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("\(presentation.title)。\(presentation.body)")
    }

    private func label(_ p: ErrorPresentation.Primary) -> String {
        switch p {
        case .retry: ErrorCopy.retry
        case .sendNow: ErrorCopy.sendNow
        case .signIn: ErrorCopy.signIn
        }
    }

    private func label(_ s: ErrorPresentation.Secondary) -> String {
        switch s {
        case .discard: ErrorCopy.discard
        case .dismiss: ErrorCopy.dismiss
        }
    }
}
