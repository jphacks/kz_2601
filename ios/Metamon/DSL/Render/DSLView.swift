import SwiftUI
import MetamonDSL
import MetamonServices

/// Component → SwiftUI View。2段固定(Container > Leaf)なので再帰は1段だけ。
struct DSLView: View {
    let component: Component
    let ctx: RenderContext

    var body: some View {
        switch component.type {
        case .vstack, .hstack, .section: ContainerView(c: component, ctx: ctx)
        case .text: TextLeaf(c: component)
        case .button: ButtonLeaf(c: component, ctx: ctx)
        case .textField: TextFieldLeaf(c: component, ctx: ctx)
        case .numberField: NumberFieldLeaf(c: component, ctx: ctx)
        case .datePicker: DatePickerLeaf(c: component, ctx: ctx)
        case .picker: PickerLeaf(c: component, ctx: ctx)
        case .toggle: ToggleLeaf(c: component, ctx: ctx)
        case .photoField: PhotoFieldLeaf(c: component, ctx: ctx)
        case .image: ImageLeaf(c: component, ctx: ctx)
        case .list: ListLeaf(c: component, ctx: ctx)
        case .stat: StatLeaf(c: component)
        case .chart: ChartLeaf(c: component)
        case .voiceInput: VoiceInputView(prompt: component.prompt)
        case .spacer: SpacerLeaf(c: component)
        case .divider: Divider()
        case .unknown: UnsupportedComponent(label: "未対応: \(component.rawType)")
        }
    }
}

/// 未知ノード。DEBUG では薄く表示、RELEASE では何も描かない(前方互換の約束)。
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
    let services = BuildConfig.make(environment: .mock)
    let rt = AppRuntime(document: (try? DSLExamples.document("kakeibo")) ?? .fallback, services: services)
    return RootView().environment(rt).environment(\.services, services)
}
