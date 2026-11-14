import SwiftUI
import UIKit

/// ホーム画面のアイコンを選ぶ(#232)。iOS の代替アイコン(`setAlternateIconName`)を使う。
/// 選んだアイコンは iOS が覚えるので、こちらで保存しない。全員が使える(Pro 不要・審査用の版にも出す)。
/// 絵の差し替え方: docs/ios/alternate-icons.md
struct AppIconOption: Identifiable, Hashable {
    /// asset catalog の appiconset の名前。
    let id: String
    /// `setAlternateIconName` に渡す名前。nil = いつものアイコン。
    let alternateName: String?

    /// 候補。並びがそのまま画面の並び。代替の名前は project.yml の `ASSETCATALOG_COMPILER_ALTERNATE_APPICON_NAMES` と揃える。
    static let all: [AppIconOption] = [
        AppIconOption(id: "AppIcon", alternateName: nil),
        AppIconOption(id: "AppIcon-Dark", alternateName: "AppIcon-Dark"),
        AppIconOption(id: "AppIcon-Light", alternateName: "AppIcon-Light"),
        AppIconOption(id: "AppIcon-Mono", alternateName: "AppIcon-Mono"),
    ]

    /// 画面のプレビュー(AppIconPreview*.imageset)。appiconset は UIImage(named:) で読めないので別に持つ(scripts/app-icon-previews.sh が作る)。
    var previewName: String { "AppIconPreview" + id.dropFirst("AppIcon".count) }

    var title: String {
        switch id {
        case "AppIcon-Dark": L10n.tr("ダーク")
        case "AppIcon-Light": L10n.tr("ライト")
        case "AppIcon-Mono": L10n.tr("モノクロ")
        default: L10n.tr("いつもの")
        }
    }
}

/// 代替アイコンの読み書き。テストでは差し替える。
@MainActor
protocol AlternateIconSetting: AnyObject {
    var supportsAlternateIcons: Bool { get }
    var alternateIconName: String? { get }
    func setAlternateIconName(_ alternateIconName: String?) async throws
}

extension UIApplication: AlternateIconSetting {}

@MainActor
@Observable
final class AppIconPickerModel {
    let options: [AppIconOption]
    /// 今のアイコン(`AppIconOption.id`)。
    private(set) var currentID: String
    private(set) var isChanging = false
    /// 変えられなかった時の文言。アラートで出す。
    var errorMessage: String?
    var isSupported: Bool { setting.supportsAlternateIcons }

    private let setting: AlternateIconSetting

    init(setting: AlternateIconSetting, options: [AppIconOption] = AppIconOption.all) {
        self.setting = setting
        self.options = options
        currentID = Self.id(for: setting.alternateIconName, in: options)
    }

    /// 候補に無い名前(古い版で選んだ等)は「いつもの」扱い。
    static func id(for alternateName: String?, in options: [AppIconOption]) -> String {
        (options.first { $0.alternateName == alternateName } ?? options.first { $0.alternateName == nil } ?? options[0]).id
    }

    func isCurrent(_ option: AppIconOption) -> Bool { option.id == currentID }

    func select(_ option: AppIconOption) async {
        guard !isChanging, option.id != currentID else { return }
        guard setting.supportsAlternateIcons else {
            errorMessage = L10n.tr("この端末ではアイコンを変えられません。")
            return
        }
        isChanging = true
        defer { isChanging = false }
        do {
            try await setting.setAlternateIconName(option.alternateName)
            currentID = option.id
        } catch {
            errorMessage = L10n.tr("アイコンを変えられませんでした。少し待ってからもう一度お試しください。")
            currentID = Self.id(for: setting.alternateIconName, in: options)
        }
    }
}

/// 設定の行。
struct AppIconSettingsRow: View {
    var body: some View {
        NavigationLink {
            AppIconPickerView(model: AppIconPickerModel(setting: UIApplication.shared))
        } label: {
            Label("アイコン", systemImage: "app")
        }
        .accessibilityIdentifier("settings.appIcon")
    }
}

struct AppIconPickerView: View {
    @State var model: AppIconPickerModel
    @ScaledMetric(relativeTo: .body) private var iconSize: CGFloat = 72

    var body: some View {
        ScrollView {
            LazyVGrid(columns: [GridItem(.adaptive(minimum: iconSize + Theme.Spacing.md), spacing: Theme.Spacing.md)],
                      spacing: Theme.Spacing.lg) {
                ForEach(model.options) { option in
                    cell(option)
                }
            }
            .padding(Theme.Spacing.md)
            Text(model.isSupported ? "変えると iOS の確認が出ます。" : "この端末ではアイコンを変えられません。")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, Theme.Spacing.md)
        }
        .background(Theme.Colors.canvas)
        .navigationTitle("アイコン")
        .navigationBarTitleDisplayMode(.inline)
        .alert("アイコンを変えられませんでした",
               isPresented: Binding(get: { model.errorMessage != nil }, set: { if !$0 { model.errorMessage = nil } })) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(model.errorMessage ?? "")
        }
    }

    private func cell(_ option: AppIconOption) -> some View {
        let selected = model.isCurrent(option)
        return Button {
            Task { await model.select(option) }
        } label: {
            VStack(spacing: Theme.Spacing.sm) {
                preview(option)
                    .frame(width: iconSize, height: iconSize)
                    .clipShape(RoundedRectangle(cornerRadius: iconSize * 0.225, style: .continuous))
                    .overlay {
                        RoundedRectangle(cornerRadius: iconSize * 0.225, style: .continuous)
                            .strokeBorder(selected ? Theme.Colors.brandA : Theme.Colors.hairline, lineWidth: selected ? 3 : 1)
                    }
                    .overlay(alignment: .bottomTrailing) {
                        if selected {
                            Image(systemName: "checkmark.circle.fill")
                                .font(.title3.weight(.semibold))
                                .symbolRenderingMode(.palette)
                                .foregroundStyle(.white, Theme.Colors.brandA)
                                .offset(x: Theme.Spacing.xs, y: Theme.Spacing.xs)
                        }
                    }
                Text(option.title)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(selected ? Theme.Colors.brandText : .primary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .disabled(model.isChanging || !model.isSupported)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(L10n.tr("アイコン: \(option.title)"))
        .accessibilityAddTraits(selected ? .isSelected : [])
        .accessibilityIdentifier("settings.appIcon.\(option.id)")
    }

    @ViewBuilder
    private func preview(_ option: AppIconOption) -> some View {
        if let image = UIImage(named: option.previewName) {
            Image(uiImage: image).resizable().scaledToFill()
        } else {
            Theme.Colors.surfaceElevated
                .overlay { Image(systemName: "app").font(.title).foregroundStyle(.secondary) }
        }
    }
}
