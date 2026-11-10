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
        /// 今日の上限(#280・429 rate_limited)。再試行ボタンは出さない
        case rateLimited(RateLimitInfo)
        /// 運営が AI を一時停止中(#511・503 ai_paused)
        case aiPaused
        /// 無料プランの一時停止(#317・503 free_plan_paused)。もう一度では解けないので再試行は出さない
        case freePlanPaused(resumeAt: Date?)
        /// プランの上限(#317・403 plan_limit_reached)。作成者だけ「Pro にする」(押した時だけ課金画面・U3/U4)
        case planLimit(PlanLimit)
        /// 持てるアプリの数の上限(#630・402 app_limit_reached)。「置き換える」で置き換えシート → 同じ発話でやり直す
        case appLimit(AppLimit)
        /// タイムアウト(45s / 504)
        case timeout
        /// 401(再取得しても失効)
        case sessionExpired
        /// 4xx(入力不備)。言い直してもらう(同じ発話の再送はしない)
        case rejected
        /// その他。message は `ErrorCopy` の決まった文(サーバーの文は入れない・H9)
        case failed(message: String)
        /// 生成以外の消えない知らせ(H9: 録音の失敗・起動時に届かない等)。主ボタンなし・「閉じる」だけ
        case notice(icon: String, title: String, body: String)
    }
    /// signInApple = 匿名の上限(M3・「Apple でサインイン」で上限が増える)/ upgradePro = Pro で上限が増える(M3)
    enum Primary: Equatable { case retry, sendNow, signIn, signInApple, upgradePro, replaceApp }
    enum Secondary: Equatable { case discard, dismiss }

    /// 何の失敗か。generate = 生成(「もう一度」で同じ発話を再生成)、chat = 生成前の対話(#75・「もう一度」で同じ履歴を再送)
    enum Origin: Equatable { case generate, chat }

    let id = UUID()
    var kind: Kind
    /// 再送に使う発話
    var transcript: String
    var origin: Origin = .generate

    /// 消えない知らせ(H9)。生成の発話は持たない
    static func notice(icon: String = "exclamationmark.triangle", title: String, body: String) -> ErrorPresentation {
        ErrorPresentation(kind: .notice(icon: icon, title: title, body: body), transcript: "")
    }

    /// 録音・文字起こしの失敗(H9)。音声画面の状態カードに出す。秒数・dB などの診断はログにだけ残す。
    /// `error` が nil なら「録音はできたが文字にならなかった」。
    static func voiceFailure(_ error: Error?, diagnostics: TranscriberDiagnostics? = nil) -> ErrorPresentation {
        if let diagnostics { ErrorLog.record(diagnostics.summary, context: "voice diagnostics") }
        guard let error else { return .notice(icon: "mic.slash", title: ErrorCopy.notHeardTitle, body: ErrorCopy.notHeardBody) }
        ErrorLog.record(error, context: "voice")
        if let f = error as? TranscriberFailure {
            return .notice(icon: "mic.slash", title: ErrorCopy.transcriberTitle(f), body: ErrorCopy.transcriberFailure(f))
        }
        return .notice(icon: "mic.slash", title: L10n.tr("文字起こしできませんでした"), body: ErrorCopy.nextStep(for: error))
    }

    static func == (a: ErrorPresentation, b: ErrorPresentation) -> Bool {
        a.kind == b.kind && a.transcript == b.transcript && a.origin == b.origin
    }

    /// `FailureKind`(Services の分類)→ 表示。`conflict` はカードにせず呼び出し側がバナー + 最新取り直しで処理する。
    static func from(_ failure: FailureKind, transcript: String, origin: Origin = .generate) -> ErrorPresentation? {
        switch failure {
        case .offline: return ErrorPresentation(kind: .offlineInterrupted, transcript: transcript, origin: origin)
        case .timeout: return ErrorPresentation(kind: .timeout, transcript: transcript, origin: origin)
        case .busy(let after): return ErrorPresentation(kind: .busy(retryAfter: after), transcript: transcript, origin: origin)
        case .sessionExpired: return ErrorPresentation(kind: .sessionExpired, transcript: transcript, origin: origin)
        case .rateLimited(let info): return ErrorPresentation(kind: .rateLimited(info), transcript: transcript, origin: origin)
        case .aiPaused: return ErrorPresentation(kind: .aiPaused, transcript: transcript, origin: origin)
        case .freePlanPaused(let at): return ErrorPresentation(kind: .freePlanPaused(resumeAt: at), transcript: transcript, origin: origin)
        case .planLimit(let limit): return ErrorPresentation(kind: .planLimit(limit), transcript: transcript, origin: origin)
        case .appLimit(let limit): return ErrorPresentation(kind: .appLimit(limit), transcript: transcript, origin: origin)
        case .appStopped: return .notice(icon: "pause.circle", title: ErrorCopy.appStoppedTitle, body: ErrorCopy.appStoppedBody)
        case .appCheckFailed: return ErrorPresentation(kind: .failed(message: ErrorCopy.appCheckFailedBody), transcript: transcript, origin: origin)
        case .rejected(let m):
            ErrorLog.record(m, context: "\(origin == .chat ? "chat" : "morph") rejected") // サーバーの文はログにだけ
            return ErrorPresentation(kind: .rejected, transcript: transcript, origin: origin)
        case .other(let m):
            ErrorLog.record(m, context: origin == .chat ? "chat" : "morph")
            return ErrorPresentation(kind: .failed(message: ErrorCopy.otherBody), transcript: transcript, origin: origin)
        case .conflict, .cancelled: return nil
        }
    }

    var icon: String {
        switch kind {
        case .offlineQueued: "wifi.slash"
        case .offlineInterrupted: "wifi.exclamationmark"
        case .busy: "hourglass"
        case .rateLimited: "moon.zzz"
        case .aiPaused, .freePlanPaused: "pause.circle"
        case .planLimit, .appLimit: "tray.full"
        case .timeout: "clock.badge.exclamationmark"
        case .sessionExpired: "person.crop.circle.badge.exclamationmark"
        case .failed, .rejected: "exclamationmark.triangle"
        case .notice(let icon, _, _): icon
        }
    }

    var title: String {
        switch kind {
        case .offlineQueued: ErrorCopy.offlineQueuedTitle
        case .offlineInterrupted: ErrorCopy.offlineInterruptedTitle
        case .busy: ErrorCopy.busyTitle
        case .rateLimited(let info): info.scope == .global ? ErrorCopy.rateLimitedGlobalTitle : ErrorCopy.rateLimitedTitle
        case .aiPaused: ErrorCopy.aiPausedTitle
        case .freePlanPaused: ErrorCopy.freePlanPausedTitle
        case .planLimit: ErrorCopy.planLimitTitle
        case .appLimit: AppLimitCopy.cardTitle
        case .timeout: ErrorCopy.timeoutTitle
        case .sessionExpired: ErrorCopy.sessionExpiredTitle
        case .failed: origin == .chat ? ErrorCopy.chatFailedTitle : ErrorCopy.failedTitle
        case .rejected: ErrorCopy.rejectedTitle
        case .notice(_, let title, _): title
        }
    }

    var body: String {
        switch kind {
        case .offlineQueued: ErrorCopy.offlineQueuedBody
        case .offlineInterrupted: ErrorCopy.offlineInterruptedBody
        case .busy(let after): after.map { $0 > 1 ? ErrorCopy.busyBody(retryAfter: $0) : ErrorCopy.busyBody } ?? ErrorCopy.busyBody
        case .rateLimited(let info): ErrorCopy.rateLimitedBody(info)
        case .aiPaused: ErrorCopy.aiPausedBody
        case .freePlanPaused(let at): ErrorCopy.freePlanPausedBody(resumeAt: at)
        case .planLimit(let limit): ErrorCopy.planLimitBody(limit)
        case .appLimit(let limit): AppLimitCopy.cardBody(limit)
        case .timeout: ErrorCopy.timeoutBody
        case .sessionExpired: ErrorCopy.sessionExpiredBody
        case .failed(let message): message
        case .rejected: ErrorCopy.rejectedBody
        case .notice(_, _, let body): body
        }
    }

    var primary: Primary? {
        switch kind {
        case .offlineQueued: .sendNow
        case .offlineInterrupted, .busy, .timeout, .failed, .aiPaused: .retry
        case .sessionExpired: .signIn
        case .rejected, .notice, .freePlanPaused: nil
        case .planLimit(let limit): ErrorCopy.showsProPath(limit) ? .upgradePro : nil
        case .appLimit: .replaceApp
        case .rateLimited(let info): Self.rateLimitPrimary(info)
        }
    }

    /// 上限のカードの次の一手(M3)。もう一度では解けないので再試行は出さない。全体の停止は待つしかない
    static func rateLimitPrimary(_ info: RateLimitInfo) -> Primary? {
        switch info.scope {
        case .global: return nil
        case .ip: return .signInApple
        case .user:
            if info.isAnonymous { return .signInApple }
            #if APP_STORE_REVIEW
            return nil // 審査用の版は App 内課金を持たない(#450 G1)
            #else
            return info.tier == "pro" ? nil : .upgradePro
            #endif
        }
    }

    var secondary: Secondary {
        switch kind {
        case .offlineQueued: .discard
        default: .dismiss
        }
    }

    /// VoiceOver で読み上げる文(H8)。カードの見出しと本文
    var announcement: String { "\(title)。\(body)" }

    var isWarning: Bool {
        if case .failed = kind { return true }
        if case .rejected = kind { return true }
        if case .notice = kind { return true }
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
            CraneMascot(pose: .panic, size: compact ? 52 : 68, sway: 3) // #102
        }
        .metamonCard(padding: compact ? Theme.Spacing.md : Theme.Spacing.md + Theme.Spacing.xs)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(presentation.announcement)
    }

    private func label(_ p: ErrorPresentation.Primary) -> String {
        switch p {
        case .retry: ErrorCopy.retry
        case .sendNow: ErrorCopy.sendNow
        case .signIn: ErrorCopy.signIn
        case .signInApple: ErrorCopy.signInApple
        case .upgradePro: ErrorCopy.upgradePro
        case .replaceApp: AppLimitCopy.replaceButton
        }
    }

    private func label(_ s: ErrorPresentation.Secondary) -> String {
        switch s {
        case .discard: ErrorCopy.discard
        case .dismiss: ErrorCopy.dismiss
        }
    }
}
