import SwiftUI
import MapKit
import PhotosUI

// MARK: - Text

struct TextComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: TextProps
    let ctx: RenderContext

    var body: some View {
        Text(rt.string(props.text, ctx))
            .font(font)
            .fontWeight(props.bold == true ? .bold : nil)
            .foregroundStyle(Color(hex: props.color) ?? .primary)
            .multilineTextAlignment(textAlignment)
            .frame(maxWidth: .infinity, alignment: frameAlignment)
    }

    private var font: Font {
        switch props.style ?? .body {
        case .largeTitle: return .largeTitle.bold()
        case .title: return .title.bold()
        case .title2: return .title2.bold()
        case .headline: return .headline
        case .body: return .body
        case .caption: return .caption
        case .footnote: return .footnote
        }
    }

    private var textAlignment: TextAlignment {
        switch props.align ?? .leading {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }

    private var frameAlignment: Alignment {
        switch props.align ?? .leading {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }
}

// MARK: - Image

struct ImageComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: ImageProps
    let ctx: RenderContext

    var body: some View {
        Group {
            if let name = props.systemName {
                Image(systemName: name)
                    .resizable()
                    .scaledToFit()
                    .foregroundStyle(.tint)
            } else if let url = URL(string: rt.string(props.url, ctx)) {
                AsyncImage(url: url) { image in
                    image.resizable().scaledToFill()
                } placeholder: {
                    Rectangle().fill(.quaternary)
                }
            }
        }
        .frame(height: CGFloat(props.height ?? 160))
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: CGFloat(props.cornerRadius ?? 12)))
    }
}

// MARK: - Button

struct ButtonComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: ButtonProps
    let ctx: RenderContext

    var body: some View {
        styled(
            Button {
                guard let action = props.action else { return }
                Task { await rt.run(action, ctx) }
            } label: {
                label.frame(maxWidth: props.style == .plain ? nil : .infinity)
            }
        )
        .disabled(props.disabled.map { rt.resolve($0, ctx).truthy } ?? false)
    }

    @ViewBuilder
    private var label: some View {
        let text = rt.string(props.label, ctx)
        if let icon = props.icon, !text.isEmpty {
            Label(text, systemImage: icon)
        } else if let icon = props.icon {
            Image(systemName: icon)
        } else {
            Text(text)
        }
    }

    @ViewBuilder
    private func styled<V: View>(_ button: V) -> some View {
        switch props.style ?? .primary {
        case .primary: button.buttonStyle(.borderedProminent).controlSize(.large)
        case .secondary: button.buttonStyle(.bordered).controlSize(.large)
        case .destructive: button.buttonStyle(.bordered).controlSize(.large).tint(.red)
        case .plain: button.buttonStyle(.plain)
        }
    }
}

// MARK: - Inputs

struct TextFieldComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: TextFieldProps
    let ctx: RenderContext

    var body: some View {
        let placeholder = rt.string(props.placeholder, ctx)
        Group {
            if props.secure == true {
                SecureField(placeholder, text: rt.stringBinding(props.bind))
            } else {
                TextField(placeholder, text: rt.stringBinding(props.bind), axis: props.multiline == true ? .vertical : .horizontal)
                    .lineLimit(props.multiline == true ? 3 ... 8 : 1 ... 1)
            }
        }
        .textFieldStyle(.roundedBorder)
        .keyboardType(keyboard)
    }

    private var keyboard: UIKeyboardType {
        switch props.keyboard {
        case "number": return .numberPad
        case "decimal": return .decimalPad
        case "email": return .emailAddress
        case "url": return .URL
        case "phone": return .phonePad
        default: return .default
        }
    }
}

struct ToggleComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: ToggleProps
    let ctx: RenderContext

    var body: some View {
        Toggle(rt.string(props.label, ctx), isOn: rt.boolBinding(props.bind))
    }
}

struct PickerComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: PickerProps
    let ctx: RenderContext

    var body: some View {
        let picker = Picker(rt.string(props.label, ctx), selection: rt.binding(props.bind)) {
            ForEach(props.options, id: \.value) { option in
                Text(option.label).tag(option.value)
            }
        }
        if props.options.count <= 4 {
            picker.pickerStyle(.segmented)
        } else {
            picker.pickerStyle(.menu)
        }
    }
}

// MARK: - Containers

struct StackComponent: View {
    let props: StackProps
    let ctx: RenderContext

    var body: some View {
        let spacing = props.spacing.map { CGFloat($0) } ?? 12
        switch props.axis {
        case .vstack:
            VStack(alignment: horizontal, spacing: spacing) { children }
        case .hstack:
            HStack(alignment: vertical, spacing: spacing) { children }
        case .zstack:
            ZStack { children }
        }
    }

    private var children: some View {
        ForEach(props.children) { DSLView(node: $0, ctx: ctx) }
    }

    private var horizontal: HorizontalAlignment {
        switch props.alignment { case "center": .center; case "trailing": .trailing; default: .leading }
    }

    private var vertical: VerticalAlignment {
        switch props.alignment { case "top": .top; case "bottom": .bottom; default: .center }
    }
}

struct ListComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: ListProps
    let ctx: RenderContext

    var body: some View {
        let rows = rt.resolve(props.source, ctx).arrayValue ?? []
        if rows.isEmpty {
            Text(rt.string(props.emptyText, ctx).isEmpty ? "まだありません" : rt.string(props.emptyText, ctx))
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 24)
        } else {
            LazyVStack(spacing: 8) {
                ForEach(Array(rows.enumerated()), id: \.offset) { index, row in
                    let rowCtx = ctx.with(item: row, index: index)
                    DSLView(node: props.item, ctx: rowCtx)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            guard let action = props.onTap else { return }
                            Task { await rt.run(action, rowCtx) }
                        }
                        .contextMenu {
                            if let delete = props.onDelete {
                                Button("削除", systemImage: "trash", role: .destructive) {
                                    Task { await rt.run(delete, rowCtx) }
                                }
                            }
                        }
                }
            }
        }
    }
}

struct CardComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: CardProps
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let title = props.title {
                Text(rt.string(title, ctx)).font(.headline)
            }
            ForEach(props.children) { DSLView(node: $0, ctx: ctx) }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 16))
        .contentShape(RoundedRectangle(cornerRadius: 16))
        .onTapGesture {
            guard let action = props.action else { return }
            Task { await rt.run(action, ctx) }
        }
    }
}

struct SectionComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: SectionProps
    let ctx: RenderContext

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let title = props.title {
                Text(rt.string(title, ctx).uppercased())
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .padding(.horizontal, 4)
            }
            ForEach(props.children) { DSLView(node: $0, ctx: ctx) }
            if let footer = props.footer {
                Text(rt.string(footer, ctx)).font(.footnote).foregroundStyle(.secondary).padding(.horizontal, 4)
            }
        }
    }
}

// MARK: - Misc

struct ProgressComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: ProgressProps
    let ctx: RenderContext

    var body: some View {
        let label = rt.string(props.label, ctx)
        if let v = props.value {
            ProgressView(value: min(max(rt.resolve(v, ctx).doubleValue ?? 0, 0), 1)) {
                if !label.isEmpty { Text(label) }
            }
        } else {
            ProgressView {
                if !label.isEmpty { Text(label) }
            }
            .frame(maxWidth: .infinity)
        }
    }
}

struct MapComponent: View {
    let props: MapProps

    var body: some View {
        let center = CLLocationCoordinate2D(latitude: props.lat, longitude: props.lng)
        let span = props.span ?? 0.02
        Map(initialPosition: .region(MKCoordinateRegion(center: center, span: .init(latitudeDelta: span, longitudeDelta: span)))) {
            ForEach(props.pins) { pin in
                Marker(pin.title ?? "", coordinate: CLLocationCoordinate2D(latitude: pin.lat, longitude: pin.lng))
            }
        }
        .frame(height: 220)
        .clipShape(RoundedRectangle(cornerRadius: 16))
    }
}

/// P0 カメラ。v0 は PhotosPicker(シミュレータでも動く)。実機では UIImagePickerController(.camera) に差し替える。
struct CameraComponent: View {
    @Environment(AppRuntime.self) private var rt
    let props: CameraProps
    @State private var selection: PhotosPickerItem?

    var body: some View {
        VStack(spacing: 8) {
            if let b64 = rt.state[path: Interpolator.statePath(props.bind)]?.stringValue,
               let data = Data(base64Encoded: b64), let image = UIImage(data: data) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
                    .frame(height: 200)
                    .frame(maxWidth: .infinity)
                    .clipShape(RoundedRectangle(cornerRadius: 16))
            }
            PhotosPicker(selection: $selection, matching: .images) {
                Label(props.label ?? "写真を撮る / 選ぶ", systemImage: "camera")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .controlSize(.large)
        }
        .onChange(of: selection) { _, item in
            guard let item else { return }
            Task {
                if let data = try? await item.loadTransferable(type: Data.self),
                   let jpeg = UIImage(data: data)?.jpegData(compressionQuality: 0.6) {
                    rt.state.set(path: Interpolator.statePath(props.bind), to: .string(jpeg.base64EncodedString()))
                }
            }
        }
    }
}

// MARK: - Helpers

extension Color {
    /// "#RRGGBB" / "#RRGGBBAA" / 名前(red, blue...)
    init?(hex: String?) {
        guard let hex else { return nil }
        switch hex {
        case "primary": self = .primary; return
        case "secondary": self = .secondary; return
        case "red": self = .red; return
        case "green": self = .green; return
        case "blue": self = .blue; return
        case "orange": self = .orange; return
        default: break
        }
        var s = hex.trimmingCharacters(in: .whitespaces)
        if s.hasPrefix("#") { s.removeFirst() }
        guard s.count == 6 || s.count == 8, let v = UInt64(s, radix: 16) else { return nil }
        let r, g, b, a: Double
        if s.count == 8 {
            r = Double((v >> 24) & 0xFF) / 255; g = Double((v >> 16) & 0xFF) / 255
            b = Double((v >> 8) & 0xFF) / 255; a = Double(v & 0xFF) / 255
        } else {
            r = Double((v >> 16) & 0xFF) / 255; g = Double((v >> 8) & 0xFF) / 255
            b = Double(v & 0xFF) / 255; a = 1
        }
        self = Color(red: r, green: g, blue: b, opacity: a)
    }
}
