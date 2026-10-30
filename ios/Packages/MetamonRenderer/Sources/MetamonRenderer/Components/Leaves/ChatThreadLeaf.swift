// 審査用ビルド(APP_STORE_REVIEW・#451 G2)ではバイナリから外す(他の人と記録を見る作り。1.2 の通報・ブロックが無い)。
#if !APP_STORE_REVIEW
import SwiftUI
import PhotosUI
import MetamonDSL

/// `chatThread`(#260): 吹き出しを古い順に上から下へ積み(最新が一番下)、自分は右・相手は左に送信者名付き。
/// 下に固定の入力欄があり、送信 = `entity` に 1 件保存(フォームを開かない・ローカル DB に即反映して同期キューが送る)。
/// 他の人のメッセージはホストの records の購読(SSE)で届き、来たら一番下までスクロールする。
/// 器の内側で自前でスクロールするので `GridScreenLayout.scrollsInside` には入れない。
struct ChatThreadLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.fabFrame) private var fabFrame
    let c: Component
    let ctx: RenderContext

    @State private var draft = ""
    @State private var photo: String?
    @State private var librarySelection: PhotosPickerItem?
    @State private var showLibrary = false
    @State private var sending = false
    @State private var barFrame: CGRect = .zero
    @State private var scrollFrame: CGRect = .zero
    @FocusState private var focused: Bool

    var body: some View {
        if let entityId = c.entity, let textField = c.textField, let entity = rt.dsl.entity(entityId) {
            VStack(spacing: RenderTheme.Spacing.sm) {
                messages(entity: entity, textField: textField)
                inputBar(entity: entity, textField: textField)
            }
        } else {
            UnsupportedComponent(label: "chatThread")
        }
    }

    // MARK: - 吹き出し

    @ViewBuilder
    private func messages(entity: Entity, textField: String) -> some View {
        let rows = ChatThread.ordered(rt.viewRecords(for: entity.id))
        let uid = rt.currentUserId
        if rows.isEmpty {
            EmptyPlaceholder(icon: "bubble.left.and.bubble.right", emptyText: c.emptyText ?? L("まだメッセージはありません"),
                             state: rt.emptyState(for: entity.id)) {
                Task { await rt.load(entities: [entity.id], force: true) }
            }
            .frame(maxWidth: .infinity, minHeight: 200, maxHeight: .infinity)
        } else {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(spacing: RenderTheme.Spacing.xs) {
                        ForEach(Array(rows.enumerated()), id: \.element.id) { i, row in
                            ChatBubble(
                                record: row,
                                text: row[textField].stringValue ?? "",
                                photo: c.photoField.map { row[$0] } ?? .null,
                                mine: ChatThread.isMine(row, in: entity, uid: uid),
                                sender: ChatThread.showsSender(row, after: i > 0 ? rows[i - 1] : nil, in: entity, uid: uid)
                                    ? (row.authorName ?? L("匿名")) : nil
                            )
                            .id(row.id)
                        }
                    }
                    .padding(.vertical, RenderTheme.Spacing.xs)
                    .padding(.bottom, FabClearance.contentMargin(viewport: scrollFrame, fab: fabFrame))
                }
                .frame(minHeight: 200) // area 無しで積まれた時も潰れない
                // 上の padding(.bottom): マイク FAB が吹き出しの下に浮く時、最後の行(時刻)を FAB の上まで持ち上げる(#157 と同じ)
                .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { scrollFrame = $0 }
                .defaultScrollAnchor(.bottom)
                .scrollDismissesKeyboard(.interactively)
                .onChange(of: rows.last?.id) { _, last in
                    guard let last else { return }
                    withAnimation(RenderTheme.Motion.appear) { proxy.scrollTo(last, anchor: .bottom) }
                }
                .onChange(of: FabClearance.contentMargin(viewport: scrollFrame, fab: fabFrame)) { _, _ in
                    // FAB の位置が後から決まった時も最新を FAB の上に出す
                    if let last = rows.last?.id { proxy.scrollTo(last, anchor: .bottom) }
                }
                .onChange(of: focused) { _, isFocused in
                    // キーボードで見える範囲が縮んでも最新を見せる
                    if isFocused, let last = rows.last?.id { proxy.scrollTo(last, anchor: .bottom) }
                }
            }
            .accessibilityIdentifier("chatThread.messages")
        }
    }

    // MARK: - 入力欄(画面の下に固定)

    private func inputBar(entity: Entity, textField: String) -> some View {
        HStack(alignment: .bottom, spacing: RenderTheme.Spacing.sm) {
            if c.photoField != nil {
                Button { showLibrary = true } label: {
                    Image(systemName: "photo").font(.title3).frame(width: 36, height: 44)
                }
                .buttonStyle(.plain)
                .foregroundStyle(RenderTheme.Colors.brandText)
                .accessibilityLabel(L("写真を選ぶ"))
                .accessibilityIdentifier("chatThread.photo")
            }
            VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
                if let photo {
                    attachedPhoto(photo)
                }
                TextField(c.placeholder ?? L("メッセージ"), text: $draft, axis: .vertical)
                    .lineLimit(1...4)
                    .textFieldStyle(.plain)
                    .font(RenderTheme.Fonts.inputValue)
                    .focused($focused)
                    .keyboardAnchor(c, $focused)
                    .accessibilityIdentifier("chatThread.input")
            }
            .renderInputChrome(focused: focused)
            Button { send(entity: entity, textField: textField) } label: {
                Image(systemName: "arrow.up.circle.fill").font(.system(size: 34)).frame(height: 44)
            }
            .buttonStyle(.plain)
            .foregroundStyle(canSend ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(.tertiary))
            .disabled(!canSend)
            .accessibilityLabel(L("送信"))
            .accessibilityIdentifier("chatThread.send")
        }
        // マイク FAB が入力欄の左に重なる時だけ、その分を空ける(キーボード表示中は FAB が隠れるので 0)
        .padding(.leading, ChatInputInset.leading(bar: barFrame, fab: fabFrame))
        .onGeometryChange(for: CGRect.self) { $0.frame(in: .global) } action: { barFrame = $0 }
        // iOS 26 のキーボード上の「完了」が送信ボタンに重ならないよう、入力中だけ持ち上げる
        .padding(.bottom, focused ? KeyboardState.toolbarClearance : 0)
        .photosPicker(isPresented: $showLibrary, selection: $librarySelection, matching: .images)
        .onChange(of: librarySelection) { _, item in
            guard let item else { return }
            Task {
                if let data = try? await item.loadTransferable(type: Data.self) { photo = PhotoCodec.dataURL(from: data) }
                librarySelection = nil
            }
        }
    }

    private func attachedPhoto(_ value: String) -> some View {
        ZStack(alignment: .topTrailing) {
            PhotoView(value: .string(value), height: 72, accessibilityLabel: L("添付する写真"))
                .frame(width: 96)
                .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
            Button { photo = nil } label: {
                Image(systemName: "xmark.circle.fill").font(.title3).symbolRenderingMode(.palette)
                    .foregroundStyle(.white, .black.opacity(0.6))
            }
            .buttonStyle(.plain)
            .padding(4)
            .accessibilityLabel(L("写真を外す"))
        }
    }

    private var canSend: Bool {
        !sending && (!draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || photo != nil)
    }

    /// 送る 1 件に入れる他の field の既定値(defaultValue を持つ保存 field だけ)。
    /// LocationTrackerLeaf.baseData と同じ中身だが、あちらは審査用ビルド(APP_STORE_REVIEW)で外れるのでここに持つ。
    static func defaultData(_ entity: Entity, skipping: [String]) -> [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity.fields where f.computed == nil && f.defaultValue != nil && !skipping.contains(f.id) {
            out[f.id] = FieldCoding.defaultValue(for: f)
        }
        return out
    }

    private func send(entity: Entity, textField: String) {
        let skip = [textField, c.photoField].compactMap { $0 }
        guard let data = ChatThread.message(text: draft, photo: photo, textField: textField, photoField: c.photoField,
                                            base: Self.defaultData(entity, skipping: skip)) else { return }
        sending = true
        let rt = rt
        Task {
            if await rt.post(entity: entity.id, data: data) {
                draft = ""
                photo = nil
            }
            sending = false
        }
    }
}

/// 1 通の吹き出し。自分は右にブランド色・相手は左に面の色(上に送信者名)、下に送った時刻。
private struct ChatBubble: View {
    let record: Record
    let text: String
    let photo: JSONValue
    let mine: Bool
    let sender: String?

    var body: some View {
        HStack(alignment: .bottom, spacing: 0) {
            if mine { Spacer(minLength: 56) }
            VStack(alignment: mine ? .trailing : .leading, spacing: 3) {
                if let sender {
                    Text(sender).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.top, RenderTheme.Spacing.xs)
                }
                bubble
                if let time {
                    Text(time).font(.caption2.monospacedDigit()).foregroundStyle(.secondary).padding(.horizontal, 6)
                }
            }
            if !mine { Spacer(minLength: 56) }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(accessibilityText)
        .accessibilityIdentifier(mine ? "chatThread.message.mine" : "chatThread.message.other")
    }

    private var hasPhoto: Bool {
        PhotoCodec.image(from: photo) != nil || (photo.stringValue?.hasPrefix("http") ?? false)
    }

    private var bubble: some View {
        let shape = RoundedRectangle(cornerRadius: 18, style: .continuous)
        return VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            if hasPhoto {
                PhotoView(value: photo, height: 160, accessibilityLabel: L("写真"))
                    .frame(maxWidth: 220)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            }
            if !text.isEmpty {
                Text(text).font(.body).foregroundStyle(mine ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 9)
        .background(mine ? AnyShapeStyle(RenderTheme.brandFill) : AnyShapeStyle(RenderTheme.Colors.surfaceElevated), in: shape)
        .overlay { if !mine { shape.strokeBorder(RenderTheme.Colors.hairline) } }
    }

    /// 今日なら「10:05」、それ以前は「10/24 10:05」。
    private var time: String? {
        guard let date = ChatThread.date(record.createdAt) else { return nil }
        let locale = LanguagePreference.shared.locale
        let style: Date.FormatStyle = Calendar.current.isDateInToday(date)
            ? .dateTime.hour().minute() : .dateTime.month(.defaultDigits).day().hour().minute()
        return date.formatted(style.locale(locale))
    }

    private var accessibilityText: String {
        let who = mine ? L("自分") : (sender ?? L("相手"))
        let body = text.isEmpty && hasPhoto ? L("写真") : text
        return [who, body, time].compactMap { $0 }.joined(separator: ", ")
    }
}

/// マイク FAB(左下に浮く)が入力欄に重なる時に左へ空ける幅(#260)。重ならない・FAB が無い(キーボード表示中など)なら 0。
enum ChatInputInset {
    static let gap: CGFloat = RenderTheme.Spacing.sm

    static func leading(bar: CGRect, fab: CGRect?, gap: CGFloat = gap) -> CGFloat {
        guard let fab, !bar.isEmpty, !fab.isEmpty,
              fab.minY < bar.maxY, fab.maxY > bar.minY,      // 縦に重なる
              fab.maxX > bar.minX, fab.minX < bar.midX       // 入力欄の左半分にかかる
        else { return 0 }
        return min((fab.maxX - bar.minX + gap).rounded(.up), bar.width / 2)
    }
}
#endif
