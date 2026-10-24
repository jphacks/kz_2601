import SwiftUI
import MetamonServices

/// 生成前の対話(#75)。初期(音声)画面で最初の発話のあとに出る。
/// - 吹き出し: 右 = ユーザー(音声の書き起こし / 文字)、左 = AI の質問。最新の質問だけ大きく
/// - 質問には選択肢チップ(`suggestions`)、ready には要約カード +「この内容で作る」
/// - 「このまま作る」は質問中も常に出す(go-ahead を送って即生成)
/// - 入力はマイク(WhisperKit・録音は親の `VoiceInputView` が持つ)+ 文字入力
struct ClarifyChatView: View {
    @Environment(AppRuntime.self) private var rt
    let micMode: MicButton.Mode
    let statusText: String
    let onMic: () -> Void
    let onSend: (String) -> Void

    @State private var typed = ""
    @FocusState private var typing: Bool

    private var ready: ChatTurn? { rt.chatTurn?.kind == .ready ? rt.chatTurn : nil }
    private var busy: Bool { rt.chatPending || rt.isMorphing }
    private var lastAssistantId: UUID? { rt.chatBubbles.last(where: { $0.role == .assistant })?.id }

    var body: some View {
        VStack(spacing: Theme.Spacing.sm) {
            transcriptScroll
            if let card = rt.errorCard {
                ErrorCard(presentation: card)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
            suggestionChips
            actionButton
            inputRow
        }
        .padding(.top, Theme.Spacing.sm)
        #if DEBUG
        .task {
            // ヘッドレス E2E 用: `METAMON_AUTO_FOCUS=chat` で 3 秒後に返答欄へフォーカス(キーボードの確認・#143)
            guard ProcessInfo.processInfo.environment["METAMON_AUTO_FOCUS"] == "chat" else { return }
            try? await Task.sleep(for: .seconds(3))
            typing = true
        }
        #endif
    }

    // MARK: - 吹き出し

    private var transcriptScroll: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(spacing: Theme.Spacing.md) {
                    ForEach(rt.chatBubbles) { bubble in
                        BubbleRow(bubble: bubble, emphasized: bubble.id == lastAssistantId && ready == nil)
                            .id(bubble.id)
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                    if rt.chatPending {
                        TypingIndicator()
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .id("pending")
                            .transition(.opacity)
                    }
                    if let ready, let brief = ready.brief {
                        ReadyCard(brief: brief)
                            .id("ready")
                            .transition(.move(edge: .bottom).combined(with: .opacity))
                    }
                }
                .padding(.vertical, Theme.Spacing.sm)
            }
            .scrollDismissesKeyboard(.interactively)
            .onChange(of: rt.chatBubbles.count) { scrollToEnd(proxy) }
            .onChange(of: rt.chatPending) { scrollToEnd(proxy) }
        }
    }

    private func scrollToEnd(_ proxy: ScrollViewProxy) {
        let target: AnyHashable? = ready != nil ? "ready" : (rt.chatPending ? "pending" : rt.chatBubbles.last.map { AnyHashable($0.id) })
        guard let target else { return }
        withAnimation(.easeOut(duration: 0.3)) { proxy.scrollTo(target, anchor: .bottom) }
    }

    // MARK: - 選択肢・ボタン

    @ViewBuilder
    private var suggestionChips: some View {
        if let turn = rt.chatTurn, turn.kind == .question, let chips = turn.suggestions, !chips.isEmpty, !busy {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: Theme.Spacing.sm) {
                    ForEach(chips, id: \.self) { chip in
                        Button { onSend(chip) } label: {
                            Text(chip)
                                .font(.subheadline.weight(.medium))
                                .padding(.horizontal, Theme.Spacing.md)
                                .padding(.vertical, Theme.Spacing.sm + 2)
                                .background(Theme.Colors.surface, in: Capsule())
                                .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.35)))
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, 1)
            }
            .transition(.opacity)
        }
    }

    /// ready なら「この内容で作る」(主)、質問中は「このまま作る」(副・常に出す)。
    @ViewBuilder
    private var actionButton: some View {
        if ready != nil {
            Button { Task { await rt.buildFromChat() } } label: {
                Label(ClarifyCopy.buildButton, systemImage: "wand.and.stars")
                    .font(.headline)
                    .foregroundStyle(.white)
                    .frame(maxWidth: .infinity)
                    .frame(height: 52)
                    .background(Theme.brandGradient, in: Capsule())
            }
            .buttonStyle(.plain)
            .disabled(busy)
            .accessibilityIdentifier("chat.build")
        } else {
            Button { Task { await rt.buildNow() } } label: {
                Label(ClarifyCopy.buildNowButton, systemImage: "forward.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(Theme.Colors.brandA)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
                    .background(Theme.Colors.surface, in: Capsule())
                    .overlay(Capsule().strokeBorder(Theme.Colors.brandA.opacity(0.4)))
            }
            .buttonStyle(.plain)
            .disabled(busy)
            .opacity(busy ? 0.5 : 1)
            .accessibilityIdentifier("chat.buildNow")
        }
    }

    // MARK: - 入力(マイク + 文字)

    private var inputRow: some View {
        HStack(spacing: Theme.Spacing.sm) {
            MicButton(mode: micMode, size: 34, action: onMic)
                .frame(width: 56, height: 56)
                .accessibilityHint(statusText)
            TextField(ClarifyCopy.replyPlaceholder, text: $typed)
                .focused($typing)
                .submitLabel(.send)
                .onSubmit(sendTyped)
                .padding(.horizontal, Theme.Spacing.md)
                .frame(height: 44)
                .background(Theme.Colors.surface, in: Capsule())
                .overlay(Capsule().strokeBorder(Theme.Colors.hairline))
            Button(action: sendTyped) {
                Image(systemName: "arrow.up")
                    .font(.body.weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: 44, height: 44)
                    .background(typed.isEmpty || busy ? AnyShapeStyle(Color.gray.opacity(0.4)) : AnyShapeStyle(Theme.brandGradient), in: Circle())
            }
            .disabled(typed.isEmpty || busy)
            .accessibilityLabel("送信")
        }
        .overlay(alignment: .topLeading) {
            if micMode == .listening {
                Text(statusText)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(Theme.Colors.recordA)
                    .offset(y: -18)
            }
        }
    }

    private func sendTyped() {
        let text = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !busy else { return }
        typed = ""
        typing = false
        onSend(text)
    }
}

// MARK: - Parts

private struct BubbleRow: View {
    let bubble: AppRuntime.ChatBubble
    /// 最新の質問(読みやすく大きく)
    let emphasized: Bool

    var body: some View {
        HStack {
            if bubble.role == .user { Spacer(minLength: 48) }
            if bubble.role == .assistant {
                Image(systemName: "sparkles")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(Theme.brandGradient)
                    .frame(width: 24, height: 24)
                    .background(Theme.Colors.surface, in: Circle())
                    .frame(maxHeight: .infinity, alignment: .top)
            }
            Text(bubble.text)
                .font(emphasized ? .title3.weight(.semibold) : .body)
                .foregroundStyle(bubble.role == .user ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
                .padding(.horizontal, Theme.Spacing.md)
                .padding(.vertical, Theme.Spacing.sm + 2)
                .background(background, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
                .fixedSize(horizontal: false, vertical: true)
            if bubble.role == .assistant { Spacer(minLength: 32) }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel((bubble.role == .user ? L10n.tr("あなた: ") : "AI: ") + bubble.text)
    }

    private var background: AnyShapeStyle {
        bubble.role == .user ? AnyShapeStyle(Theme.brandGradient) : AnyShapeStyle(Theme.Colors.surface)
    }
}

/// ready の要約(brief)。「この内容で作ります」+ アプリ名・目的・記録する項目・見たいもの。
private struct ReadyCard: View {
    let brief: ChatBrief

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.sm) {
            Label(ClarifyCopy.readyTitle, systemImage: "checkmark.seal.fill")
                .font(.caption.weight(.semibold))
                .foregroundStyle(Theme.brandGradient)
            Text(brief.name).font(.title3.weight(.bold))
            if !brief.purpose.isEmpty {
                Text(brief.purpose).font(.subheadline).foregroundStyle(.secondary)
            }
            row(L10n.tr("記録する"), brief.entities)
            row(L10n.tr("見られる"), brief.features)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .metamonCard()
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private func row(_ label: String, _ items: [String]) -> some View {
        if !items.isEmpty {
            HStack(alignment: .firstTextBaseline, spacing: Theme.Spacing.sm) {
                Text(label).font(.caption.weight(.semibold)).foregroundStyle(.tertiary).frame(width: 52, alignment: .leading)
                Text(items.joined(separator: L10n.tr("・"))).font(.subheadline)
            }
        }
    }
}

/// 応答待ちの「…」。
private struct TypingIndicator: View {
    var body: some View {
        TimelineView(.animation(minimumInterval: 1.0 / 20)) { tl in
            let t = tl.date.timeIntervalSinceReferenceDate
            HStack(spacing: 5) {
                ForEach(0..<3) { i in
                    Circle()
                        .fill(Color.secondary)
                        .frame(width: 7, height: 7)
                        .opacity(0.35 + 0.65 * max(0, sin(t * 5 - Double(i) * 0.7)))
                }
            }
            .padding(.horizontal, Theme.Spacing.md)
            .padding(.vertical, Theme.Spacing.sm + 4)
            .background(Theme.Colors.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
            .padding(.leading, 32)
        }
        .accessibilityLabel(ClarifyCopy.thinking)
    }
}
