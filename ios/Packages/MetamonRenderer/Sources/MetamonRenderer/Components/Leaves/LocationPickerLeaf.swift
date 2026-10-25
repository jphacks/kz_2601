import SwiftUI
import MapKit
import MetamonDSL

/// `locationPicker`(#212): 現在地以外の場所を選ぶ入力欄。タップでシートを開き、
/// 店名・地名・住所で検索(MKLocalSearch)/ 地図を動かして中央のピン / 現在地 から選ぶ。
/// 保存は locationButton と同じ "lat,lng"。`nameField` があれば場所の名前も入れる。
/// フォームには選んだ場所の名前(無ければ座標)と小さな地図を出す。
struct LocationPickerLeaf: View {
    let c: Component
    let ctx: RenderContext
    @State private var showSheet = false
    /// nameField が無い時に表示だけで使う、直前に選んだ場所の名前
    @State private var pickedName: String?

    var body: some View {
        if let form = ctx.form, let field = c.field {
            content(form: form, field: field)
        } else {
            NeedsForm(c: c)
        }
    }

    @ViewBuilder
    private func content(form: FormDraft, field: String) -> some View {
        let coord = PlacePicking.coordinate(form.values[field])
        VStack(alignment: .leading, spacing: 8) {
            if let coord {
                HStack(spacing: 6) {
                    Image(systemName: "mappin.circle.fill").foregroundStyle(.tint)
                    Text(displayName(form: form, field: field))
                        .font(.subheadline.weight(.semibold))
                        .lineLimit(2)
                        .accessibilityIdentifier("locationPicker.value")
                }
                Map(initialPosition: .region(MKCoordinateRegion(center: coord, latitudinalMeters: 800, longitudinalMeters: 800)),
                    interactionModes: []) {
                    Marker("", coordinate: coord)
                }
                .id(form.values[field]?.stringValue)
                .frame(height: 110)
                .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
                .allowsHitTesting(false)
                .accessibilityHidden(true)
            }
            Button { showSheet = true } label: {
                Label(coord == nil ? (c.label ?? L("場所を選ぶ")) : L("場所を変える"), systemImage: "magnifyingglass")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .accessibilityIdentifier("locationPicker.open")
        }
        .sheet(isPresented: $showSheet) {
            LocationPickerSheet(title: c.label ?? L("場所を選ぶ"), initial: coord) { place in
                PlacePicking.apply(place, to: &form.values, field: field, nameField: c.nameField)
                pickedName = place.name
            }
        }
    }

    private func displayName(form: FormDraft, field: String) -> String {
        if let nf = c.nameField, let s = form.values[nf]?.stringValue, !s.isEmpty { return s }
        if let pickedName, !pickedName.isEmpty { return pickedName }
        return form.values[field]?.stringValue ?? ""
    }
}

/// 場所を選ぶシート: 検索欄 + (検索中は結果一覧 / それ以外は地図と中央のピン)+ 現在地 / この場所に決める。
struct LocationPickerSheet: View {
    let title: String
    let initial: CLLocationCoordinate2D?
    let onPick: (PickedPlace) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var query = ""
    @State private var results: [PickedPlace] = []
    @State private var searching = false
    @State private var position: MapCameraPosition
    @State private var center: CLLocationCoordinate2D
    @State private var region: MKCoordinateRegion?
    @State private var locating = false
    @State private var error: String?

    init(title: String, initial: CLLocationCoordinate2D?, onPick: @escaping (PickedPlace) -> Void) {
        self.title = title
        self.initial = initial
        self.onPick = onPick
        // 初期位置: 選択済みの場所 / モックは金沢駅 / それ以外は東京駅
        let start = initial ?? (PlacePicking.isMock ? PlacePicking.mockHere.coordinate
                                                    : CLLocationCoordinate2D(latitude: 35.681236, longitude: 139.767125))
        _center = State(initialValue: start)
        _position = State(initialValue: .region(MKCoordinateRegion(center: start, latitudinalMeters: 2_000, longitudinalMeters: 2_000)))
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                searchField
                if query.trimmingCharacters(in: .whitespaces).isEmpty { mapArea } else { resultList }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L("キャンセル")) { dismiss() }
                }
            }
            .task(id: query) {
                let q = query
                guard !q.trimmingCharacters(in: .whitespaces).isEmpty else { results = []; return }
                searching = true
                try? await Task.sleep(for: .milliseconds(300)) // 打鍵ごとに検索しない
                guard !Task.isCancelled else { return }
                let found = await PlacePicking.search(q, near: region)
                guard !Task.isCancelled else { return }
                results = found
                searching = false
            }
        }
    }

    private var searchField: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
            TextField(L("店名・地名・住所で検索"), text: $query)
                .submitLabel(.search)
                .autocorrectionDisabled()
                .accessibilityIdentifier("locationPicker.search")
            if !query.isEmpty {
                Button { query = "" } label: { Image(systemName: "xmark.circle.fill").foregroundStyle(.secondary) }
                    .accessibilityLabel(L("検索を消す"))
            }
        }
        .padding(10)
        .background(.quaternary.opacity(0.6), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    @ViewBuilder
    private var resultList: some View {
        if results.isEmpty {
            VStack {
                Spacer()
                if searching { ProgressView() } else { Text(L("見つかりませんでした")).foregroundStyle(.secondary) }
                Spacer()
            }
            .frame(maxWidth: .infinity)
        } else {
            List(Array(results.enumerated()), id: \.element.id) { i, place in
                Button { pick(place) } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(place.name ?? place.locationValue).foregroundStyle(.primary)
                        if let address = place.address, !address.isEmpty {
                            Text(address).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                        }
                    }
                }
                .accessibilityIdentifier("locationPicker.result.\(i)")
            }
            .listStyle(.plain)
        }
    }

    private var mapArea: some View {
        VStack(spacing: 0) {
            ZStack {
                Map(position: $position)
                    .onMapCameraChange(frequency: .continuous) { ctx in
                        center = ctx.region.center
                        region = ctx.region
                    }
                // 中央のピン(先端が地図の中心に来るよう上にずらす)
                Image(systemName: "mappin")
                    .font(.system(size: 36, weight: .bold))
                    .foregroundStyle(.red)
                    .shadow(radius: 2)
                    .offset(y: -18)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
            VStack(spacing: 8) {
                Text(L("地図を動かして、ピンの位置で決められます"))
                    .font(.caption).foregroundStyle(.secondary)
                if let error { Text(error).font(.caption).foregroundStyle(.red) }
                HStack(spacing: 12) {
                    Button { Task { await locate() } } label: {
                        HStack {
                            if locating { ProgressView().controlSize(.small) }
                            Label(L("現在地"), systemImage: "location.fill")
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .secondary))
                    .disabled(locating)
                    .accessibilityIdentifier("locationPicker.current")
                    Button { pick(PickedPlace(name: nil, latitude: center.latitude, longitude: center.longitude)) } label: {
                        Label(L("この場所に決める"), systemImage: "checkmark").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(RenderButtonStyle(kind: .primary))
                    .accessibilityIdentifier("locationPicker.usePin")
                }
            }
            .padding(16)
        }
    }

    private func pick(_ place: PickedPlace) {
        onPick(place)
        dismiss()
    }

    /// 現在地へ地図を動かす(決めるのは「この場所に決める」)。モックは金沢駅。
    private func locate() async {
        locating = true
        defer { locating = false }
        let here: CLLocationCoordinate2D
        if PlacePicking.isMock {
            here = PlacePicking.mockHere.coordinate
        } else {
            do { here = try await OneShotLocation().request().coordinate } catch {
                self.error = L("位置情報を取得できませんでした(\(error.localizedDescription))")
                return
            }
        }
        error = nil
        center = here
        withAnimation { position = .region(MKCoordinateRegion(center: here, latitudinalMeters: 800, longitudinalMeters: 800)) }
    }
}
