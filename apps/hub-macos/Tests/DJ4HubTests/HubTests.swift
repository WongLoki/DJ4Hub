import XCTest
import NativeAudio
@testable import DJ4Hub

final class HubTests: XCTestCase {
    func testIncomingAlertsDoNotReplayOldMessagesOrRepeatedCalls() {
        var tracker = IncomingAlertTracker()
        XCTAssertFalse(tracker.update(sms: ["old"], ringing: []).sms)
        let next = tracker.update(sms: ["old", "new"], ringing: ["call"])
        XCTAssertTrue(next.sms); XCTAssertTrue(next.call)
        let repeated = tracker.update(sms: ["new"], ringing: ["call"])
        XCTAssertFalse(repeated.sms); XCTAssertFalse(repeated.call)
        _ = tracker.update(sms: [], ringing: [])
        XCTAssertFalse(tracker.update(sms: ["old"], ringing: ["call"]).call)
        XCTAssertTrue(tracker.update(sms: [], ringing: ["next-call"]).call)
    }
    func testCarrierDisplayPrefersNameAndRetainsCode() {
        XCTAssertEqual(CarrierDisplay(raw: " 53001 ").name, "One NZ")
        XCTAssertEqual(CarrierDisplay(raw: "53001").code, "53001")
        XCTAssertEqual(CarrierDisplay(raw: "53004").name, "One NZ")
        XCTAssertEqual(CarrierDisplay(raw: "中国移动").name, "中国移动")
        XCTAssertNil(CarrierDisplay(raw: "中国移动").code)
        XCTAssertEqual(CarrierDisplay(raw: "99999").name, "未知运营商")
        XCTAssertEqual(CarrierDisplay(raw: "99999").code, "99999")
        XCTAssertEqual(CarrierDisplay(raw: "").name, "未读取到")
    }
    func testMessageSelectionIdentitySurvivesPolling() {
        let message = HubValue(["sender": "Example", "timestamp": "2026-09-12", "content": "Test"])
        XCTAssertEqual(SMSRecord(value: message).id, SMSRecord(value: message).id)
        XCTAssertNotEqual(SMSRecord(value: message).id, SMSRecord(value: HubValue(["sender": "Example", "timestamp": "2026-09-12", "content": "Another"])).id)
    }
    func testProfilesKeepCardApplicationInIdentity() {
        let value = HubValue(["iccid": "test", "state": 1])
        XCTAssertNotEqual(ProfileRecord(aid: "a", value: value).id, ProfileRecord(aid: "b", value: value).id)
        XCTAssertTrue(ProfileRecord(aid: "a", value: value).enabled)
    }
    func testInterfaceRowsHandleEmptyAddresses() {
        let row = InterfaceRecord(HubValue(["name": "en9", "ipv4": "", "status": "inactive", "kind": "ethernet"]))
        XCTAssertEqual(row.id, "en9")
        XCTAssertEqual(row.address, "—")
        XCTAssertEqual(row.state, "未活跃")
    }
    func testTrafficHistoryUsesDeltasAndClearsOnDisconnect() {
        var history = TrafficHistory()
        func sample(_ time: Int, _ rx: Int, _ tx: Int, _ interface: String = "en9") -> HubValue {
            HubValue(["available": true, "sampled_at_ms": time, "rx_bytes": rx, "tx_bytes": tx, "interface": interface])
        }
        history.record(sample(1000, 100, 50))
        XCTAssertTrue(history.download.isEmpty)
        history.record(sample(6000, 1100, 550))
        XCTAssertEqual(history.download, [200]); XCTAssertEqual(history.upload, [100])
        history.record(sample(11000, 2100, 1050, "en11"))
        XCTAssertTrue(history.download.isEmpty)
        history.record(sample(16000, 3100, 1550, "en11"))
        XCTAssertEqual(history.download, [200])
        history.record(HubValue(["available": false]))
        XCTAssertTrue(history.download.isEmpty)
    }
    func testTrafficHistoryDoesNotGraphCounterResetsOrLongGaps() {
        var history = TrafficHistory()
        func sample(_ time: Int, _ bytes: Int) -> HubValue { HubValue(["available": true, "sampled_at_ms": time, "rx_bytes": bytes, "tx_bytes": bytes, "interface": "en9"]) }
        history.record(sample(1000, 500)); history.record(sample(6000, 10))
        XCTAssertTrue(history.download.isEmpty)
        history.record(sample(30000, 1000))
        XCTAssertTrue(history.download.isEmpty)
    }
    func testAudioRejectsEmptyDevicesWithoutOpeningMicrophone() {
        var error: Int32 = 0
        XCTAssertNil(dj_audio_start("", "", &error))
        XCTAssertEqual(error, -50)
    }
    func testMissingFieldsAndArrays() {
        let value = HubValue(["calls": [["state": 0]], "ok": true])
        XCTAssertEqual(value["missing"].text, "—")
        XCTAssertTrue(value["ok"].bool)
        XCTAssertEqual(value["calls"].array[0]["state"].text, "0")
    }
    func testPagesRemainNativeAndComplete() {
        XCTAssertEqual(HubPage.allCases.count, 7)
        XCTAssertEqual(Set(HubPage.allCases.map(\.symbol)).count, 7)
    }
}
