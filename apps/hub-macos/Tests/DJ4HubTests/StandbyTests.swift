import XCTest
import Foundation
@testable import DJ4Hub

private final class StandbyProtocol: URLProtocol {
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data("{\"token\":\"test-standby\",\"active\":true}".utf8))
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}

final class StandbyTests: XCTestCase {
    @MainActor func testStandbyDoesNotConnectComputerAudioAndCanRelease() async throws {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StandbyProtocol.self]
        let session = URLSession(configuration: config)
        defer { session.invalidateAndCancel() }
        let service = HubService(session: session)
        let voice = NativeVoice()
        try await voice.prepareStandby(service)
        XCTAssertTrue(voice.prepared)
        XCTAssertFalse(voice.connected)
        XCTAssertFalse(voice.busy)
        try await voice.prepareStandby(service)
        XCTAssertTrue(voice.prepared)
        XCTAssertFalse(voice.connected)
        await voice.release()
        XCTAssertFalse(voice.prepared)
        XCTAssertFalse(voice.connected)
    }
}
