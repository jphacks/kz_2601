import XCTest
@testable import MetamonDSL

/// 近くの人への送り物(NearbyShare)の判定: 曲を受けられるか・同じアプリか・id の長さの端・項目の形の並び(下書き stock7-ios-010)。
final class NearbyPayloadDecisionTests: XCTestCase {
    private var savedLanguage: String?
    override func setUp() { savedLanguage = setAppLanguage("ja") }
    override func tearDown() { setAppLanguage(savedLanguage) }

    private let song = MusicSong(id: "abc.1", title: "曲", artist: "歌手")

    private func songHeader(appId: String) -> NearbyShare.Header {
        NearbyShare.Header(sender: "花子", appId: appId, appName: "x", song: song)
    }

    func testDecideSongAcceptsOnlyWhenAppUsesRequests() throws {
        let music = try ExamplesFixture.load("drive_music")
        let plain = try ExamplesFixture.load("kakeibo")
        XCTAssertEqual(NearbyShare.decideSong(song, dsl: music, sameApp: true), .accept(entityId: NearbyShare.songEntityId, sameApp: true))
        XCTAssertEqual(NearbyShare.decideSong(song, dsl: music, sameApp: false), .accept(entityId: NearbyShare.songEntityId, sameApp: false))
        XCTAssertEqual(NearbyShare.decideSong(song, dsl: plain, sameApp: true),
                       .reject(reason: "このアプリは曲のリクエストを受け付けていません"))
    }

    func testDecideSongRejectsBlankTitle() throws {
        let music = try ExamplesFixture.load("drive_music")
        let blank = MusicSong(title: "  \n", artist: "歌手")
        XCTAssertEqual(NearbyShare.decideSong(blank, dsl: music, sameApp: true),
                       .reject(reason: "曲名のないリクエストは受け取れません"))
    }

    func testDecideRoutesSongHeaderBySameAppFlag() throws {
        let music = try ExamplesFixture.load("drive_music")
        let h = songHeader(appId: "a1")
        XCTAssertEqual(NearbyShare.decide(h, dsl: music, appId: "a1"), .accept(entityId: NearbyShare.songEntityId, sameApp: true))
        XCTAssertEqual(NearbyShare.decide(h, dsl: music, appId: "a2"), .accept(entityId: NearbyShare.songEntityId, sameApp: false))
    }

    func testDecideRejectsNewerVersionBeforeAnythingElse() throws {
        let music = try ExamplesFixture.load("drive_music")
        var h = songHeader(appId: "a1")
        h.v = NearbyShare.version + 1
        XCTAssertEqual(NearbyShare.decide(h, dsl: music, appId: "a1"), .reject(reason: "相手のアプリが新しすぎるため受け取れません"))
        h.v = NearbyShare.version
        XCTAssertEqual(NearbyShare.decide(h, dsl: music, appId: "a1"), .accept(entityId: NearbyShare.songEntityId, sameApp: true))
        h.v = 0
        XCTAssertEqual(NearbyShare.decide(h, dsl: music, appId: "a1"), .accept(entityId: NearbyShare.songEntityId, sameApp: true), "古い版は受ける")
    }

    func testIsValidIdBoundaries() {
        XCTAssertFalse(NearbyLimits.isValidId(""))
        XCTAssertTrue(NearbyLimits.isValidId("a"))
        XCTAssertTrue(NearbyLimits.isValidId(String(repeating: "a", count: NearbyLimits.maxIdLength)))
        XCTAssertFalse(NearbyLimits.isValidId(String(repeating: "a", count: NearbyLimits.maxIdLength + 1)))
        // 文字数で数える(バイト数ではない)
        XCTAssertTrue(NearbyLimits.isValidId(String(repeating: "あ", count: NearbyLimits.maxIdLength)))
    }

    func testValidatedRejectsLongIdsAndTooManyFields() throws {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        let entity = try XCTUnwrap(dsl.entity("attendance"))
        let ok = NearbyShare.Header(sender: "s", appId: "a", appName: "x", entity: entity, count: 1)
        XCTAssertNotNil(ok.validated())

        var longApp = ok
        longApp.appId = String(repeating: "a", count: NearbyLimits.maxIdLength + 1)
        XCTAssertNil(longApp.validated())

        var emptyEntity = ok
        emptyEntity.entityId = ""
        XCTAssertNil(emptyEntity.validated())

        var badFieldId = ok
        badFieldId.fields = [.init(id: "", type: "string")]
        XCTAssertNil(badFieldId.validated())

        var badFieldType = ok
        badFieldType.fields = [.init(id: "n", type: String(repeating: "t", count: NearbyLimits.maxIdLength + 1))]
        XCTAssertNil(badFieldType.validated())

        var manyOk = ok
        manyOk.fields = (0..<NearbyLimits.maxFields).map { .init(id: "f\($0)", type: "string") }
        XCTAssertNotNil(manyOk.validated())
        var tooMany = manyOk
        tooMany.fields.append(.init(id: "extra", type: "string"))
        XCTAssertNil(tooMany.validated())
    }

    func testValidatedClipsNamesAndCount() throws {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        let entity = try XCTUnwrap(dsl.entity("attendance"))
        var h = NearbyShare.Header(sender: "  ", appId: "a", appName: String(repeating: "あ", count: 300), entity: entity, count: 9999)
        h.entityName = "  " + String(repeating: "い", count: 300)
        let v = try XCTUnwrap(h.validated())
        XCTAssertEqual(v.sender, NearbyLimits.anonymousName)
        XCTAssertEqual(v.appName.count, NearbyLimits.maxLabelLength)
        XCTAssertEqual(v.entityName.count, NearbyLimits.maxLabelLength)
        XCTAssertEqual(v.count, NearbyShare.maxReceiveRecords)
        h.count = -5
        XCTAssertEqual(h.validated()?.count, 0)
    }

    func testSignatureFollowsStoredFieldsInOrderAndSkipsComputed() throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"e","name":"E","fields":[
           {"id":"a","name":"A","type":"string"},
           {"id":"n","name":"N","type":"integer"},
           {"id":"c","name":"C","type":"integer","computed":{"op":"add","args":[{"ref":"n"},1]}},
           {"id":"d","name":"D","type":"date"}]}],
         "screens":[{"id":"home","title":"t","components":[]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let entity = try XCTUnwrap(dsl.entity("e"))
        XCTAssertEqual(NearbyShare.signature(of: entity),
                       [.init(id: "a", type: "string"), .init(id: "n", type: "integer"), .init(id: "d", type: "date")])
    }

    func testDecideOtherAppSignatureIgnoresOrderButNotMissingField() throws {
        let dsl = try ExamplesFixture.load("attendance_nearby")
        let entity = try XCTUnwrap(dsl.entity("attendance"))
        var h = NearbyShare.Header(sender: "s", appId: "other", appName: "x", entity: entity, count: 1)
        h.fields.reverse()
        XCTAssertEqual(NearbyShare.decide(h, dsl: dsl, appId: "mine"), .accept(entityId: "attendance", sameApp: false), "並びは問わない")
        h.fields.removeLast()
        guard case .reject = NearbyShare.decide(h, dsl: dsl, appId: "mine") else { return XCTFail("項目が欠けたら reject") }
    }
}
