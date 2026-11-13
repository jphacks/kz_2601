import XCTest
@testable import MetamonDSL

/// `OpenApp` が宛先を作る前の値の確かめ(username・phoneNumber・isEmail)を関数ごとに。
final class OpenAppValueTests: XCTestCase {
    // MARK: - username

    func testUsernameStripsLeadingAt() {
        XCTAssertEqual(OpenApp.username("@taro", max: 15, extra: "_"), "taro")
        XCTAssertEqual(OpenApp.username("taro_99", max: 15, extra: "_"), "taro_99")
        XCTAssertNil(OpenApp.username("@", max: 15, extra: "_")) // @ だけは空
        XCTAssertNil(OpenApp.username("@@taro", max: 15, extra: "_")) // 外すのは先頭の 1 つだけ
    }

    func testUsernameLengthLimit() {
        let fifteen = String(repeating: "a", count: 15)
        XCTAssertEqual(OpenApp.username(fifteen, max: 15, extra: "_"), fifteen)
        XCTAssertEqual(OpenApp.username("@" + fifteen, max: 15, extra: "_"), fifteen) // @ は数えない
        XCTAssertNil(OpenApp.username(fifteen + "a", max: 15, extra: "_"))
        XCTAssertNil(OpenApp.username("", max: 15, extra: "_"))
    }

    func testUsernameAllowedCharactersDependOnExtra() {
        XCTAssertNil(OpenApp.username("taro.k", max: 15, extra: "_")) // X は . を使えない
        XCTAssertEqual(OpenApp.username("taro.k", max: 30, extra: "._"), "taro.k") // Instagram は使える
        for bad in ["taro-k", "taro k", "たろう", "taro!", "ｔａｒｏ"] {
            XCTAssertNil(OpenApp.username(bad, max: 30, extra: "._"), bad)
        }
    }

    // MARK: - phoneNumber

    func testPhoneNumberKeepsDigitsAndLeadingPlus() {
        XCTAssertEqual(OpenApp.phoneNumber("090-1234-5678"), "09012345678")
        XCTAssertEqual(OpenApp.phoneNumber("03 1234 5678"), "0312345678")
        XCTAssertEqual(OpenApp.phoneNumber("(03) 1234-5678"), "0312345678")
        XCTAssertEqual(OpenApp.phoneNumber("+81 90-1234-5678"), "+819012345678")
        XCTAssertEqual(OpenApp.phoneNumber("110"), "110")
    }

    func testPhoneNumberFullwidth() {
        XCTAssertEqual(OpenApp.phoneNumber("０９０１２３４５６７８"), "09012345678")
        XCTAssertEqual(OpenApp.phoneNumber("＋８１　９０－１２３４－５６７８"), "+819012345678")
    }

    func testPhoneNumberRejects() {
        XCTAssertNil(OpenApp.phoneNumber("1")) // 1 桁
        XCTAssertNil(OpenApp.phoneNumber(String(repeating: "1", count: 21))) // 21 桁
        XCTAssertNil(OpenApp.phoneNumber("090-1234-5678 内線 3"))
        XCTAssertNil(OpenApp.phoneNumber("tel:0312345678"))
        XCTAssertNil(OpenApp.phoneNumber("abc"))
        XCTAssertNil(OpenApp.phoneNumber(""))
    }

    // MARK: - isEmail

    func testIsEmailAcceptsCommonForms() {
        for ok in ["taro@example.com", "a.b+tag@sub.example.co.jp", "TARO_1@example.org"] {
            XCTAssertTrue(OpenApp.isEmail(ok), ok)
        }
    }

    func testIsEmailRejectsBadForms() {
        for bad in ["taro@example", "taro.example.com", "@example.com", "taro@", "taro@@example.com",
                    "taro @example.com", "taro@exa mple.com", "taro@example.com?cc=x", "taro@ex/ample.com",
                    "taro@example.com#a", "a&b@example.com", ""] {
            XCTAssertFalse(OpenApp.isEmail(bad), bad)
        }
    }
}
