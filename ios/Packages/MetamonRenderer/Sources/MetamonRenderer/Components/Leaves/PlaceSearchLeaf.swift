import SwiftUI
import MapKit
import MetamonDSL

/// `placeSearch`: 現在地の近くの場所を Apple のマップで探す一覧(フォーム画面のみ)。
/// 「近くを探す」を押した時だけ位置を聞き(使用中のみ)、近い順に名前・距離・住所を出す。
/// 「記録に追加」で入力中のフォームの nameField / locationField / addressField を上書きする(保存は saveRecord)。
/// 位置を断られても落ち着いて案内し、語を打てば位置なしで探せる。問い合わせは端末から Apple へ(本アプリのサーバーは通らない)。
struct PlaceSearchLeaf: View {
    let c: Component
    let ctx: RenderContext

    @State private var typed = ""
    @State private var results: [(place: PickedPlace, meters: CLLocationDistance?)] = []
    @State private var here: CLLocationCoordinate2D?
    @State private var searching = false
    @State private var searched = false
    @State private var notice: String?
    /// 現在地の精度が低い時の一言(#1344)
    @State private var accuracyNote: String?
    @State private var addedId: String?

    private var fixedQuery: String? {
        guard let q = c.query?.trimmingCharacters(in: .whitespacesAndNewlines), !q.isEmpty else { return nil }
        return q
    }
    private var words: String? {
        if let fixedQuery { return fixedQuery }
        let t = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }
    private var canSearch: Bool { words != nil || PlaceSearch.category(c.category) != nil }

    var body: some View {
        if let form = ctx.form, c.nameField != nil || c.locationField != nil || c.addressField != nil {
            content(form: form)
        } else {
            NeedsForm(c: c)
        }
    }

    private func content(form: FormDraft) -> some View {
        VStack(alignment: .leading, spacing: RenderTheme.Spacing.sm) {
            Label(c.label ?? L("近くの場所"), systemImage: "location.magnifyingglass")
                .font(.headline)
                .accessibilityAddTraits(.isHeader)
            if fixedQuery == nil {
                searchField
            }
            Button { Task { await search() } } label: {
                HStack {
                    if searching { ProgressView().controlSize(.small) }
                    Label(searched ? L("もう一度探す") : L("近くを探す"), systemImage: "location.fill")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(RenderButtonStyle(kind: .secondary))
            .disabled(searching || !canSearch)
            .accessibilityIdentifier("placeSearch.go")
            if let notice {
                Text(notice).font(.caption).foregroundStyle(.secondary)
                    .accessibilityIdentifier("placeSearch.notice")
            }
            if let accuracyNote, !results.isEmpty {
                Text(accuracyNote).font(.caption).foregroundStyle(.secondary)
                    .accessibilityIdentifier("placeSearch.accuracy")
            }
            if c.pins == true, !results.isEmpty { pinMap }
            if searched, results.isEmpty, !searching, notice == nil {
                Text(L("見つかりませんでした")).font(.subheadline).foregroundStyle(.secondary)
            }
            ForEach(Array(results.enumerated()), id: \.element.place.id) { i, r in
                if i > 0 { Divider() }
                row(r.place, meters: r.meters, index: i, form: form)
            }
        }
    }

    private var searchField: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
            TextField(L("店名・種類で探す(例: ラーメン)"), text: $typed)
                .submitLabel(.search)
                .autocorrectionDisabled()
                .onSubmit {
                    #if os(macOS)
                    guard IMEReturnGuard.allowsReturn(KeyboardState.focusedTextView()) else { return }
                    #endif
                    Task { await search() }
                }
                .accessibilityIdentifier("placeSearch.search")
        }
        .padding(10)
        .background(.quaternary.opacity(0.6), in: RoundedRectangle(cornerRadius: RenderTheme.Radius.inner, style: .continuous))
    }

    private var pinMap: some View {
        Map(initialPosition: .automatic, interactionModes: PlatformMap.inlineModes([.pan, .zoom])) {
            if let here { Marker(L("現在地"), systemImage: "location.fill", coordinate: here).tint(.blue) }
            ForEach(results, id: \.place.id) { r in
                Marker(r.place.name ?? "", coordinate: r.place.coordinate)
            }
        }
        .macMapControls() // Mac: +/- で拡大縮小(#1346)
        .id(results.map(\.place.id).joined())
        .frame(height: 160)
        .clipShape(RoundedRectangle(cornerRadius: RenderTheme.Radius.card, style: .continuous))
        .accessibilityHidden(true)
    }

    private func row(_ place: PickedPlace, meters: CLLocationDistance?, index: Int, form: FormDraft) -> some View {
        let added = addedId == place.id
        return HStack(alignment: .center, spacing: RenderTheme.Spacing.sm) {
            VStack(alignment: .leading, spacing: 2) {
                Text(place.name ?? place.locationValue)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(2)
                Text([meters.map(PlaceSearch.distanceText), place.address].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " · "))
                    .font(.caption).foregroundStyle(.secondary).lineLimit(2)
            }
            .accessibilityElement(children: .combine)
            Spacer(minLength: 4)
            Button {
                PlaceSearch.apply(place, to: &form.values, nameField: c.nameField, locationField: c.locationField, addressField: c.addressField)
                addedId = place.id
            } label: {
                Label(added ? L("追加しました") : L("記録に追加"), systemImage: added ? "checkmark" : "plus")
                    .font(.caption.weight(.semibold))
            }
            .buttonStyle(.bordered)
            .accessibilityIdentifier("placeSearch.add.\(index)")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("placeSearch.result.\(index)")
    }

    private func search() async {
        guard canSearch, !searching else { return }
        searching = true
        defer { searching = false; searched = true }
        notice = nil
        accuracyNote = nil
        addedId = nil
        if PlacePicking.isMock {
            here = PlacePicking.mockHere.coordinate
        } else {
            do {
                let loc = try await OneShotLocation().request()
                here = loc.coordinate
                // 精度が低いと近い順の距離が当てにならない(#1344)
                accuracyNote = LocationAccuracy.classify(loc.horizontalAccuracy).searchNote
            } catch {
                here = nil
                // 断られても責めない。語があれば位置なしで探す
                let denied = error is OneShotLocation.LocationError
                notice = switch (denied, words == nil) {
                case (true, true): LocationPermissionCopy.cannotSearchNearby
                case (false, true): L("現在地がわからないので近くを探せません。少し待つか、探す語を入れてください")
                default: L("現在地が使えないので、近くに限らず探しました")
                }
            }
        }
        let found = await PlaceSearch.search(query: words, category: c.category, here: here)
        results = PlaceSearch.nearestFirst(found, from: here)
    }
}
