import XCTest
@testable import DJ4Hub

final class HistoryTests: XCTestCase {
    @MainActor func testHistoryQueryRemainsQuery() throws {
        let service = HubService()
        let url = try service.requestURL("api/history?kind=call&card=current&offset=100")
        XCTAssertEqual(url.path, "/api/history")
        XCTAssertEqual(url.query, "kind=call&card=current&offset=100")
        XCTAssertEqual(url.host, "127.0.0.1")
        XCTAssertEqual(try service.requestURL("api/health").path, "/api/health")
        XCTAssertThrowsError(try service.requestURL("https://example.com/api/history"))
        XCTAssertThrowsError(try service.requestURL("//example.com/api/history"))
    }
    func testPhonePresentation() {
        XCTAssertNotNil(HistoryPresentation.date("2026-09-13T10:20:30+12:00"))
        XCTAssertNotNil(HistoryPresentation.date("2026-09-13T10:20:30.123+12:00"))
        XCTAssertNil(HistoryPresentation.date("—"))
        XCTAssertEqual(HistoryPresentation.cardLabel(""), "未归属")
        XCTAssertEqual(HistoryPresentation.cardLabel("—"), "未归属")
        XCTAssertEqual(HistoryPresentation.cardLabel("12345678"), "SIM · 5678")
        XCTAssertEqual(HistoryPresentation.duration(65), "1 分 5 秒")
        XCTAssertEqual(HistoryPresentation.dialNumber("+64 (21) 123-456"), "+6421123456")
        XCTAssertNil(HistoryPresentation.dialNumber("未知号码"))
        XCTAssertNil(HistoryPresentation.dialNumber("*123#"))
    }
}
