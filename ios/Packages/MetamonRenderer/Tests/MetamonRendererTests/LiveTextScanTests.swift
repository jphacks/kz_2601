import XCTest
@testable import MetamonRenderer

/// #2228 カメラを向けて文字・数字を読む口。画面は出せないので、
/// 純関数(文字 → 数字・読んだ行 → 欲しい 1 つ)と口の差し替えを確かめる。
final class LiveTextScanTests: XCTestCase {

    // MARK: - 完了の条件の 3 例

    func testNumberFromCurrencyFullwidthAndUnit() {
        XCTAssertEqual(ScannedNumber.number(from: "¥1,234"), 1234, "通貨記号と 3 桁区切りを落とす")
        XCTAssertEqual(ScannedNumber.number(from: "１２３"), 123, "全角の数字は半角に寄せる")
        XCTAssertEqual(ScannedNumber.number(from: "12.5kg"), 12.5, "単位は捨てて小数点は残す")
    }

    // MARK: - 数字の取り出し

    func testNumberIgnoresCurrencyMarksAndUnits() {
        XCTAssertEqual(ScannedNumber.number(from: "￥1,234"), 1234, "全角の円記号")
        XCTAssertEqual(ScannedNumber.number(from: "1,234円"), 1234)
        XCTAssertEqual(ScannedNumber.number(from: "$12"), 12)
        XCTAssertEqual(ScannedNumber.number(from: "合計 ¥ 1,234"), 1234, "記号と数のあいだの空白")
        XCTAssertEqual(ScannedNumber.number(from: "80%"), 80)
        XCTAssertEqual(ScannedNumber.number(from: "36.6℃"), 36.6)
    }

    func testNumberHandlesFullwidthDigitsAndSeparators() {
        XCTAssertEqual(ScannedNumber.number(from: "１，２３４"), 1234, "全角のカンマ付き")
        XCTAssertEqual(ScannedNumber.number(from: "１２．５ｋｇ"), 12.5, "全部が全角")
        XCTAssertEqual(ScannedNumber.number(from: "1,234,567"), 1234567, "区切りが 2 つ")
    }

    func testNumberTreatsCommaAsSeparatorOnlyBeforeThreeDigits() {
        XCTAssertEqual(ScannedNumber.numbers(in: "2,5"), [2, 5], "3 桁でないカンマは区切りと見ない")
        XCTAssertEqual(ScannedNumber.numbers(in: "1,234"), [1234])
    }

    func testNumberKeepsDecimalPoint() {
        XCTAssertEqual(ScannedNumber.number(from: "0.5"), 0.5)
        XCTAssertEqual(ScannedNumber.number(from: "12."), 12, "うしろに数が無いピリオドは小数点にしない")
        XCTAssertEqual(ScannedNumber.number(from: "1,234.50"), 1234.5, "区切りと小数点が両方")
    }

    func testNumberReadsMinus() {
        XCTAssertEqual(ScannedNumber.number(from: "-1,234"), -1234)
        XCTAssertEqual(ScannedNumber.number(from: "−500円"), -500, "全角寄りのマイナス記号")
        XCTAssertEqual(ScannedNumber.number(from: "△1,000"), -1000, "会計のマイナス")
        XCTAssertEqual(ScannedNumber.number(from: "▲12.5"), -12.5)
    }

    func testNumbersReturnsEveryNumberInOrder() {
        XCTAssertEqual(ScannedNumber.numbers(in: "コーヒー 450円 ケーキ 520円"), [450, 520])
        XCTAssertEqual(ScannedNumber.numbers(in: "5-3"), [5, 3], "数のうしろの - は次の数の区切り")
        XCTAssertEqual(ScannedNumber.number(from: "小計 1,000 合計 1,100"), 1000, "最初の 1 つ")
    }

    func testNumberIsNilWithoutDigits() {
        XCTAssertNil(ScannedNumber.number(from: "合計"))
        XCTAssertNil(ScannedNumber.number(from: ""))
        XCTAssertNil(ScannedNumber.number(from: "¥"))
        XCTAssertEqual(ScannedNumber.numbers(in: "ありがとうございました"), [])
    }

    // MARK: - 読んだ行 → 欲しい 1 つ

    func testPickFindsFirstLineWithNumber() {
        let lines = ["レシート", "ありがとうございました", "合計 ¥1,234"]
        XCTAssertEqual(LiveTextScanPick.first(kind: .number, in: lines), "合計 ¥1,234")
        XCTAssertEqual(LiveTextScanPick.first(kind: .text, in: lines), "レシート")
    }

    func testPickSkipsEmptyLines() {
        XCTAssertEqual(LiveTextScanPick.first(kind: .text, in: ["", "   ", "本文"]), "本文")
        XCTAssertNil(LiveTextScanPick.first(kind: .text, in: ["", "  "]))
        XCTAssertNil(LiveTextScanPick.first(kind: .number, in: ["合計", "お買い上げ"]))
    }

    func testPickCutsOutPhoneEmailAndUrl() {
        XCTAssertEqual(LiveTextScanPick.first(kind: .phone, in: ["店名", "TEL 076-123-4567"]), "076-123-4567")
        XCTAssertEqual(LiveTextScanPick.first(kind: .email, in: ["問い合わせ: shop@example.com まで"]), "shop@example.com")
        XCTAssertEqual(LiveTextScanPick.first(kind: .url, in: ["https://example.com/shop を見てね"]), "https://example.com/shop")
        XCTAssertNil(LiveTextScanPick.first(kind: .email, in: ["合計 1,234"]))
    }

    // MARK: - 口の差し替え(使えない端末のフォールバック)

    @MainActor
    func testScanReturnsTappedText() async {
        let scanner = FakeLiveTextScanner(scanned: "合計 ¥1,234")
        let photos = FakeLiveTextPhotoSource()
        let text = await LiveTextScan.text(kind: .number, using: scanner, photoSource: photos)
        XCTAssertEqual(text, "合計 ¥1,234")
        XCTAssertEqual(scanner.lastKind, .number)
        XCTAssertEqual(photos.photoCount, 0, "カメラで読めたら写真には落ちない")
    }

    @MainActor
    func testNumberExtractsFromTappedText() async {
        let scanner = FakeLiveTextScanner(scanned: "合計 ¥1,234")
        let value = await LiveTextScan.number(using: scanner, photoSource: FakeLiveTextPhotoSource())
        XCTAssertEqual(value, 1234)
    }

    @MainActor
    func testUnsupportedDeviceFallsBackToPhoto() async {
        let scanner = FakeLiveTextScanner(isSupported: false, scanned: "読めるはずがない")
        let photos = FakeLiveTextPhotoSource()
        let text = await LiveTextScan.text(kind: .number, using: scanner, photoSource: photos)
        XCTAssertEqual(scanner.scanCount, 0, "使えない端末では読み取りの画面を出さない")
        XCTAssertEqual(photos.photoCount, 1, "写真から読む方に落ちる")
        XCTAssertNil(text, "写真が無ければ nil")
    }

    @MainActor
    func testUnavailableScannerFallsBackToPhoto() async {
        let scanner = FakeLiveTextScanner(isSupported: true, isAvailable: false)
        let photos = FakeLiveTextPhotoSource()
        _ = await LiveTextScan.text(kind: .text, using: scanner, photoSource: photos)
        XCTAssertEqual(scanner.scanCount, 0)
        XCTAssertEqual(photos.photoCount, 1)
    }

    @MainActor
    func testCancelledScanFallsBackToPhoto() async {
        let scanner = FakeLiveTextScanner(scanned: nil)
        let photos = FakeLiveTextPhotoSource()
        let value = await LiveTextScan.number(using: scanner, photoSource: photos)
        XCTAssertEqual(scanner.scanCount, 1)
        XCTAssertEqual(photos.photoCount, 1, "やめた時も写真から読む方を試す")
        XCTAssertNil(value)
    }

    @MainActor
    func testLiveScannerIsNotSupportedOnMac() {
        let scanner = LiveTextScanner()
        #if os(macOS)
        XCTAssertFalse(scanner.isSupported)
        XCTAssertFalse(scanner.isAvailable)
        #else
        _ = scanner.isSupported
        #endif
    }
}
