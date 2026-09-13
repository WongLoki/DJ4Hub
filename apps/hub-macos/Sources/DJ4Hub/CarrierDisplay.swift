import Foundation

/// Serving network, not the SIM issuer: these may differ while roaming.
struct CarrierDisplay {
    let name: String
    let code: String?

    init(raw: String) {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        // One NZ publishes MCC 530 / MNC 01; ITU also allocates 53004 to One NZ.
        // https://www.itu.int/dms_pub/itu-t/opb/sp/T-SP-OB.1280-2023-OAS-PDF-E.pdf
        let names = ["53001": "One NZ", "53004": "One NZ"]
        let numeric = value.range(of: "^[0-9]{5,6}$", options: .regularExpression) != nil
        code = numeric ? value : nil
        name = names[value] ?? (numeric ? "未知运营商" : (value.isEmpty || value == "—" ? "未读取到" : value))
    }
}
