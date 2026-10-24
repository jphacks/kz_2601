import SwiftUI

/// Node → SwiftUI View の再帰レンダラー。各 case は ComponentViews.swift の小さな View に委譲する。
struct DSLView: View {
    @Environment(AppRuntime.self) private var rt
    let node: Node
    var ctx: RenderContext = .init()

    var body: some View {
        if rt.isVisible(node, ctx) {
            content.padding(node.padding.map { CGFloat($0) } ?? 0)
        }
    }

    @ViewBuilder
    private var content: some View {
        switch node.component {
        case .text(let p): TextComponent(props: p, ctx: ctx)
        case .image(let p): ImageComponent(props: p, ctx: ctx)
        case .button(let p): ButtonComponent(props: p, ctx: ctx)
        case .textField(let p): TextFieldComponent(props: p, ctx: ctx)
        case .toggle(let p): ToggleComponent(props: p, ctx: ctx)
        case .picker(let p): PickerComponent(props: p, ctx: ctx)
        case .stack(let p): StackComponent(props: p, ctx: ctx)
        case .list(let p): ListComponent(props: p, ctx: ctx)
        case .card(let p): CardComponent(props: p, ctx: ctx)
        case .section(let p): SectionComponent(props: p, ctx: ctx)
        case .spacer(let p): Spacer(minLength: p.minLength.map { CGFloat($0) })
        case .divider: Divider()
        case .progress(let p): ProgressComponent(props: p, ctx: ctx)
        case .map(let p): MapComponent(props: p)
        case .camera(let p): CameraComponent(props: p)
        case .aiVoice(let p): VoiceInputView(prompt: p.prompt)
        case .unknown(let type): UnsupportedComponent(label: "未対応: \(type)")
        case .invalid(let type, let reason): UnsupportedComponent(label: "不正な \(type): \(reason)")
        }
    }
}

/// 未知/不正ノード。DEBUG では薄く表示、RELEASE では何も描かない。
struct UnsupportedComponent: View {
    let label: String

    var body: some View {
        #if DEBUG
        Text(label)
            .font(.caption2)
            .foregroundStyle(.secondary)
            .padding(6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.yellow.opacity(0.15), in: RoundedRectangle(cornerRadius: 6))
        #else
        EmptyView()
        #endif
    }
}

#Preview("家計簿サンプル") {
    let rt = AppRuntime(document: (try? AppDocument.bundled("app_kakeibo")) ?? .fallback, services: .mock())
    return RootView().environment(rt)
}
