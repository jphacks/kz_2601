import SwiftUI
import MapKit
import MetamonDSL

/// `routeGuide`(#411): 目的地までの道案内。地図に経路の線・次に曲がる所と距離・残りの距離・到着の予想時刻・音声案内(オン/オフ)・車/徒歩。
/// 目的地は次のどれか 1 つ(backend の検証で 1 つに決まる):
/// - `address`: 固定の住所・施設名を MapKit で検索した 1 件目
/// - `entity` + `locationField`(画面の entity と別): その entity のレコードから部品の中で選ぶ(既定は先頭)
/// - `locationField` だけ: フォーム・詳細画面の今のレコード(同じフォームの locationPicker で選んだ値をそのまま使う)
/// 案内そのものは `RouteGuideSession`。画面を離れると止まる(使用中の位置だけ)。
struct RouteGuideLeaf: View {
    @Environment(\.dslRuntime) private var rt
    @Environment(\.openURL) private var openURL
    let c: Component
    let ctx: RenderContext

    @State private var session = RouteGuideSession()
    @State private var position: MapCameraPosition = .automatic
    @State private var mode: RouteGuidance.Mode?
    @State private var selectedId: String?
    @State private var addressPlace: RouteGuidance.Place?
    @State private var addressNotFound = false
    @AppStorage("metamon.routeGuide.voice") private var voiceOn = true

    private typealias Place = RouteGuidance.Place

    private var currentMode: RouteGuidance.Mode { mode ?? RouteGuidance.Mode(dsl: c.mode) }
    private var address: String? {
        guard let a = c.address?.trimmingCharacters(in: .whitespacesAndNewlines), !a.isEmpty else { return nil }
        return a
    }
    /// 部品の中で行き先を選ぶ(entity が画面の entity と別)
    private var choosesRecord: Bool {
        guard let e = c.entity, !e.isEmpty else { return false }
        return e != ctx.form?.entity.id
    }
    private var titleField: String? {
        c.titleField ?? rt.dsl.entity(c.entity ?? ctx.form?.entity.id ?? "")?.titleField
    }

    private var candidates: [Place] {
        guard choosesRecord, let e = c.entity, let field = c.locationField else { return [] }
        return RouteGuidance.places(rt.viewRecords(for: e), locationField: field, titleField: titleField)
    }

    private var destination: Place? {
        if address != nil { return addressPlace }
        if choosesRecord {
            let all = candidates
            return all.first { $0.id == selectedId } ?? all.first
        }
        guard let form = ctx.form, let field = c.locationField else { return nil }
        return RouteGuidance.place(id: form.recordId ?? "draft", values: form.values, locationField: field, titleField: titleField)
    }

    var body: some View {
        if address == nil, c.locationField == nil || (!choosesRecord && ctx.form == nil) {
            UnsupportedComponent(label: L("routeGuide は目的地(address か locationField)が必要です"))
        } else {
            content(destination)
        }
    }

    private func content(_ dest: Place?) -> some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            if let title = c.title, !title.isEmpty {
                Text(title)
                    .font(RenderTheme.Fonts.sectionTitle)
                    .foregroundStyle(.secondary)
                    .accessibilityAddTraits(.isHeader)
            }
            header(dest)
            switch session.phase {
            case .guiding: banner
            case .arrived:
                Label(L("目的地に到着しました"), systemImage: "flag.checkered")
                    .font(.headline)
                    .foregroundStyle(RenderTheme.Colors.positive)
                    .accessibilityIdentifier("routeGuide.arrived")
            default: EmptyView()
            }
            map(dest)
            controls(dest)
            footnote(dest)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .renderCard()
        .onAppear {
            session.voiceOn = voiceOn
            position = Self.camera(around: dest)
        }
        .onDisappear { session.stop() }
        .onChange(of: voiceOn) { _, on in session.voiceOn = on }
        .onChange(of: dest?.coord) { _, _ in
            // 行き先が変わったら(別の行き先を選んだ・フォームで場所を選び直した)案内をやめる
            if session.phase != .idle { session.reset() }
            position = Self.camera(around: dest)
        }
        .onChange(of: session.route) { _, route in
            guard let route else { return }
            let fit = MapViewLeaf.fit(route.polyline.map(Self.cl), minSpan: 500)
            position = RouteGuideSession.isMock ? fit : .userLocation(followsHeading: currentMode == .automobile, fallback: fit)
        }
        .onChange(of: session.current) { _, cur in
            guard RouteGuideSession.isMock, session.phase == .guiding, let cur else { return }
            withAnimation(.easeInOut(duration: 0.4)) {
                position = .camera(MapCamera(centerCoordinate: Self.cl(cur), distance: currentMode == .automobile ? 1_800 : 700))
            }
        }
        .task(id: address) { await resolveAddress() }
    }

    // MARK: - parts

    @ViewBuilder
    private func header(_ dest: Place?) -> some View {
        let all = choosesRecord ? candidates : []
        HStack(spacing: 6) {
            Image(systemName: "flag.checkered").foregroundStyle(.tint).accessibilityHidden(true)
            if all.count > 1, !session.isActive {
                Menu {
                    ForEach(all) { p in
                        Button(name(p)) { selectedId = p.id }
                    }
                } label: {
                    HStack(spacing: 4) {
                        Text(dest.map(name) ?? "").lineLimit(1)
                        Image(systemName: "chevron.up.chevron.down").font(.caption)
                    }
                    .font(.subheadline.weight(.semibold))
                }
                .accessibilityLabel(L("行き先を選ぶ"))
                .accessibilityValue(dest.map(name) ?? "")
                .accessibilityIdentifier("routeGuide.destination")
            } else {
                Text(dest.map(name) ?? (address ?? L("目的地")))
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(1)
                    .accessibilityIdentifier("routeGuide.destination")
            }
            Spacer(minLength: 0)
        }
    }

    @ViewBuilder
    private var banner: some View {
        if let p = session.progress {
            VStack(alignment: .leading, spacing: 2) {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    Image(systemName: Self.maneuverIcon(p.nextInstruction))
                        .font(.title2.weight(.bold))
                        .foregroundStyle(.tint)
                        .accessibilityHidden(true)
                    Text(RouteGuidance.formatDistance(p.distanceToManeuver))
                        .font(.title2.weight(.bold).monospacedDigit())
                    Text(p.nextInstruction ?? L("目的地"))
                        .font(.headline)
                        .lineLimit(2)
                        .minimumScaleFactor(0.8)
                }
                HStack(spacing: 4) {
                    Text(L("残り \(RouteGuidance.formatDistance(p.remainingDistance))"))
                    Text("・")
                    Text(Self.duration(p.remainingTime))
                    Text("・")
                    Text(L("\(Self.clock(Date.now.addingTimeInterval(p.remainingTime)))着"))
                }
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("routeGuide.banner")
        }
    }

    private func map(_ dest: Place?) -> some View {
        Map(position: $position) {
            if let route = session.route {
                MapPolyline(coordinates: route.polyline.map(Self.cl))
                    .stroke(.tint, style: StrokeStyle(lineWidth: 6, lineCap: .round, lineJoin: .round))
            }
            if let dest {
                Marker(name(dest), systemImage: "flag.checkered", coordinate: Self.cl(dest.coord))
            }
            if RouteGuideSession.isMock {
                if let cur = session.current {
                    Annotation("", coordinate: Self.cl(cur)) {
                        Circle().fill(.blue).frame(width: 16, height: 16).overlay(Circle().stroke(.white, lineWidth: 3))
                    }
                }
            } else {
                UserAnnotation()
            }
        }
        .mapStyle(.standard(pointsOfInterest: .excludingAll))
        .frame(maxWidth: .infinity, minHeight: 110, maxHeight: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.control, style: .continuous))
        .accessibilityLabel(L("経路の地図"))
    }

    private func controls(_ dest: Place?) -> some View {
        HStack(spacing: RenderTheme.Spacing.sm) {
            Picker(L("移動手段"), selection: Binding(get: { currentMode }, set: { m in
                mode = m
                session.change(mode: m)
            })) {
                Text(L("車")).tag(RouteGuidance.Mode.automobile)
                Text(L("徒歩")).tag(RouteGuidance.Mode.walking)
            }
            .pickerStyle(.segmented)
            .frame(maxWidth: 120)
            .accessibilityIdentifier("routeGuide.mode")
            Button { voiceOn.toggle() } label: {
                Image(systemName: voiceOn ? "speaker.wave.2.fill" : "speaker.slash.fill")
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .frame(maxWidth: 64)
            .accessibilityLabel(voiceOn ? L("音声案内をオフにする") : L("音声案内をオンにする"))
            .accessibilityIdentifier("routeGuide.voice")
            if session.phase == .idle {
                Button {
                    guard let dest else { return }
                    session.start(to: dest.coord, mode: currentMode)
                } label: {
                    Label(c.label ?? L("案内を開始"), systemImage: "location.north.line.fill")
                }
                .buttonStyle(RenderButtonStyle(kind: .primary))
                .disabled(dest == nil)
                .accessibilityIdentifier("routeGuide.start")
            } else {
                Button { session.reset() } label: {
                    Label(L("案内を終了"), systemImage: "xmark")
                }
                .buttonStyle(RenderButtonStyle(kind: session.phase == .arrived ? .secondary : .destructive))
                .accessibilityIdentifier("routeGuide.stop")
            }
        }
    }

    @ViewBuilder
    private func footnote(_ dest: Place?) -> some View {
        Group {
            switch session.phase {
            case .locating:
                Label(L("現在地を確認しています…"), systemImage: "location")
            case .routing:
                Label(L("経路を探しています…"), systemImage: "point.topleft.down.to.point.bottomright.curvepath")
            case .denied:
                VStack(alignment: .leading, spacing: 4) {
                    Text(L("位置情報がオフのため案内できません。設定で「このAppの使用中のみ許可」を選んでください"))
                        .foregroundStyle(RenderTheme.Colors.negative)
                    Button(L("設定を開く")) {
                        if let url = URL(string: "app-settings:") { openURL(url) }
                    }
                    .font(.caption.weight(.semibold))
                }
            case .failed(let message):
                Text(message).foregroundStyle(RenderTheme.Colors.negative)
            case .guiding, .arrived:
                if let cue = session.lastCue, voiceOn {
                    Label(cue, systemImage: "speaker.wave.2")
                        .lineLimit(2)
                        .accessibilityIdentifier("routeGuide.cue")
                }
            case .idle:
                if dest == nil {
                    Text(missingText)
                } else {
                    Text(L("案内中は画面がつきっぱなしになります。運転中は画面を操作しないでください"))
                }
            }
        }
        .font(.caption)
        .foregroundStyle(.secondary)
    }

    private var missingText: String {
        if let address { return addressNotFound ? L("「\(address)」が見つかりませんでした") : L("目的地を探しています…") }
        if choosesRecord { return L("場所を登録した行き先がまだありません") }
        return L("場所を選ぶと、そこまで案内できます")
    }

    // MARK: - helpers

    private func name(_ p: Place) -> String {
        p.name.isEmpty ? (address ?? L("目的地")) : p.name
    }

    private func resolveAddress() async {
        guard let address else { return }
        addressNotFound = false
        let found = await PlacePicking.search(address, near: nil).first
        guard !Task.isCancelled else { return }
        addressPlace = found.map { Place(id: "address", name: address, coord: .init(lat: $0.latitude, lng: $0.longitude)) }
        addressNotFound = found == nil
    }

    static func cl(_ c: RouteGuidance.Coord) -> CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: c.lat, longitude: c.lng)
    }

    static func camera(around dest: RouteGuidance.Place?) -> MapCameraPosition {
        guard let dest else { return .automatic }
        return .region(MKCoordinateRegion(center: cl(dest.coord), latitudinalMeters: 1_500, longitudinalMeters: 1_500))
    }

    /// 案内文から矢印を選ぶ(MapKit の文は端末の言語なので日英の語で見る)。
    static func maneuverIcon(_ instruction: String?) -> String {
        guard let s = instruction?.lowercased() else { return "flag.checkered" }
        if s.contains("uターン") || s.contains("u-turn") { return "arrow.uturn.down" }
        if s.contains("右") || s.contains("right") { return "arrow.turn.up.right" }
        if s.contains("左") || s.contains("left") { return "arrow.turn.up.left" }
        if s.contains("到着") || s.contains("目的地") || s.contains("destination") || s.contains("arrive") { return "flag.checkered" }
        return "arrow.up"
    }

    /// 到着の予想時刻(12:34 / 12:34 PM)
    static func clock(_ date: Date) -> String {
        date.formatted(Date.FormatStyle(date: .omitted, time: .shortened).locale(LanguagePreference.shared.locale))
    }

    /// 「あと 12 分」「あと 1 時間 5 分」
    static func duration(_ seconds: TimeInterval) -> String {
        let minutes = max(1, Int((seconds / 60).rounded(.up)))
        if minutes < 60 { return L("あと\(minutes)分") }
        return L("あと\(minutes / 60)時間\(minutes % 60)分")
    }
}
