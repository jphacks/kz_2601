// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す。
#if !APP_STORE_REVIEW
import SwiftUI
import CoreImage.CIFilterBuiltins
import MetamonServices

/// 変身一覧の各カードから開くシェアシート(#49 / #50)。
/// 上: シェアリンク(QR + 送る + 取り消し)、下: 「探す」への公開 / 非公開。
struct ShareAppSheet: View {
    let app: AppSummary
    @Environment(AppRuntime.self) private var rt
    @Environment(\.services) private var services
    @Environment(\.dismiss) private var dismiss
    @State private var ui = SharingUI.shared

    @State private var share: ShareInfo?
    @State private var shareError: String?
    @State private var listing: PublicAppListing?
    @State private var listingLoaded = false
    @State private var desc = ""
    @State private var tags = ""
    @State private var busy = false
    @State private var copied = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.Spacing.lg) {
                header
                linkSection
                publishSection
            }
            .padding(Theme.Spacing.lg)
        }
        .scrollDismissesKeyboard(.interactively) // 説明・タグの入力後に下スワイプで閉じる(#143)
        .background(Theme.Colors.canvas.ignoresSafeArea())
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .task { await load() }
    }

    // MARK: - Sections

    private var header: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: Theme.Spacing.xs) {
                Text("「\(app.name)」をシェア")
                    .font(.system(size: 22, weight: .bold, design: .rounded))
                Text("受け取った人は自分のアプリとして使えます(記録は共有されません)")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            Button { dismiss() } label: {
                Image(systemName: "xmark")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(.secondary)
                    .padding(Theme.Spacing.sm + 2)
                    .background(Theme.Colors.surface, in: Circle())
            }
            .accessibilityLabel("閉じる")
            .accessibilityIdentifier("share.close")
        }
    }

    @ViewBuilder
    private var linkSection: some View {
        VStack(spacing: Theme.Spacing.md) {
            if let share, let appURL = URL(string: share.url) {
                QRCodeImage(text: share.url)
                    .frame(width: 200, height: 200)
                    .padding(Theme.Spacing.md)
                    .background(.white, in: RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
                    .accessibilityLabel("シェア用 QR コード")
                Text("カメラで読み取ると Tailor It で開きます")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                ShareLink(item: URL(string: share.webUrl) ?? appURL,
                          subject: Text(app.name),
                          message: Text("Tailor It で「\(app.name)」を使ってみて\n\(share.url)")) {
                    Label("リンクを送る", systemImage: "square.and.arrow.up")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.white)
                        .frame(maxWidth: .infinity)
                        .frame(height: 48)
                        .background(Theme.brandGradient, in: Capsule())
                }
                HStack(spacing: Theme.Spacing.md) {
                    Button {
                        UIPasteboard.general.string = share.url
                        copied = true
                    } label: {
                        Label(copied ? L10n.tr("コピーしました") : L10n.tr("リンクをコピー"), systemImage: copied ? "checkmark" : "doc.on.doc")
                    }
                    Spacer()
                    Button(role: .destructive) { Task { await revoke(share) } } label: {
                        Label("リンクを取り消す", systemImage: "link.badge.plus").labelStyle(.titleOnly)
                    }
                }
                .font(.footnote.weight(.medium))
                .disabled(busy)
            } else if let shareError {
                Label(shareError, systemImage: "exclamationmark.triangle")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                Button("もう一度") { Task { await issue() } }.buttonStyle(.bordered)
            } else {
                RoundedRectangle(cornerRadius: Theme.Radius.md)
                    .fill(Theme.Colors.surfaceElevated)
                    .frame(width: 232, height: 232)
                    .overlay(ProgressView())
                    .accessibilityLabel("リンクを作成中")
            }
        }
        .frame(maxWidth: .infinity)
        .metamonCard()
    }

    @ViewBuilder
    private var publishSection: some View {
        VStack(alignment: .leading, spacing: Theme.Spacing.md) {
            Label("みんなに公開", systemImage: "globe.asia.australia")
                .font(.headline)
            if !listingLoaded {
                RoundedRectangle(cornerRadius: 6).fill(Theme.Colors.surfaceElevated).frame(height: 44)
            } else if let listing {
                Text("「探す」に公開中・\(listing.installs) 人がインストール")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                Button(role: .destructive) { Task { await unpublish() } } label: {
                    Text("公開をやめる").frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .disabled(busy)
            } else {
                Text("「探す」から誰でも見つけてインストールできるようになります")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                // 説明は複数行(return で改行)、タグは 1 行(return で閉じる)・#196
                TextField("ひとこと説明(任意)", text: $desc, axis: .vertical)
                    .lineLimit(2...5)
                    .metamonInputField()
                TextField("タグ(スペース区切り・任意) 例: 健康 習慣", text: $tags)
                    .submitLabel(.done)
                    .metamonInputField()
                Button { Task { await publish() } } label: {
                    Text("公開する")
                        .font(.subheadline.weight(.semibold))
                        .frame(maxWidth: .infinity)
                        .frame(height: 44)
                }
                .buttonStyle(.borderedProminent)
                .disabled(busy)
            }
        }
        .metamonCard()
    }

    // MARK: - Actions

    private func load() async {
        if let cached = ui.issued[app.id] { share = cached } else { await issue() }
        if let sharing = services.sharing {
            listing = try? await sharing.searchPublicApps(query: "", sort: "new").first { $0.id == app.id && $0.mine }
        }
        listingLoaded = true
    }

    private func issue() async {
        guard let sharing = services.sharing else {
            shareError = L10n.tr("シェアはサーバー接続時のみ使えます")
            return
        }
        shareError = nil
        do {
            let s = try await sharing.share(appId: app.id)
            ui.issued[app.id] = s
            share = s
            print("[Metamon] share issued: \(s.url)")
        } catch {
            shareError = L10n.tr("リンクを作れませんでした: \(error.localizedDescription)")
        }
    }

    private func revoke(_ s: ShareInfo) async {
        busy = true
        defer { busy = false }
        do {
            try await services.sharing?.revokeShare(token: s.token)
            ui.issued[app.id] = nil
            share = nil
            rt.toast = L10n.tr("リンクを取り消しました。もう開けません")
            await issue() // 新しいリンクを用意しておく
        } catch {
            rt.toast = L10n.tr("取り消せませんでした: \(error.localizedDescription)")
        }
    }

    private func publish() async {
        busy = true
        defer { busy = false }
        let tagList = tags.split(whereSeparator: { $0 == " " || $0 == "　" || $0 == "," || $0 == "、" }).map(String.init)
        do {
            listing = try await services.sharing?.publish(appId: app.id, description: desc, tags: tagList, authorName: nil)
            rt.toast = L10n.tr("「探す」に公開しました")
        } catch {
            rt.toast = L10n.tr("公開できませんでした: \(error.localizedDescription)")
        }
    }

    private func unpublish() async {
        busy = true
        defer { busy = false }
        do {
            try await services.sharing?.unpublish(appId: app.id)
            listing = nil
            rt.toast = L10n.tr("公開をやめました")
        } catch {
            rt.toast = L10n.tr("非公開にできませんでした: \(error.localizedDescription)")
        }
    }
}

/// 文字列の QR コード(CoreImage)。拡大でぼやけないよう補間なしで描く。
struct QRCodeImage: View {
    let text: String

    var body: some View {
        if let image = Self.make(text) {
            Image(uiImage: image)
                .interpolation(.none)
                .resizable()
                .scaledToFit()
        } else {
            Image(systemName: "qrcode").resizable().scaledToFit().foregroundStyle(.secondary)
        }
    }

    static func make(_ text: String) -> UIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(text.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage?.transformed(by: CGAffineTransform(scaleX: 8, y: 8)),
              let cg = CIContext().createCGImage(output, from: output.extent) else { return nil }
        return UIImage(cgImage: cg)
    }
}
#endif
