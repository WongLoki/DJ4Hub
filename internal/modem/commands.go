package modem

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/WongLoki/dj-4g-hub/internal/apduarbiter"
	"github.com/WongLoki/dj-4g-hub/internal/simaid"
)

func (m *Manager) QueryIMEI() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CGSN", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseIMEI(resp), nil
}

func (m *Manager) QueryFirmware() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CGMR", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseFirmware(resp), nil
}

func (m *Manager) QuerySIMInserted() (bool, error) {
	if resp, err := m.ExecuteATSilent("AT+QSIMSTAT?", 2*time.Second); err == nil {
		if inserted, ok := parseQSIMSTATInserted(resp); ok {
			return inserted, nil
		}
	}
	resp, err := m.ExecuteATSilent("AT+CPIN?", 2*time.Second)
	if err != nil {
		return false, err
	}
	if inserted, ok := parseCPINInserted(resp); ok {
		return inserted, nil
	}
	return false, nil
}

func (m *Manager) QueryIMSI() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CIMI", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseIMSI(resp), nil
}

func (m *Manager) QueryICCID() (string, error) {
	resp, err := m.ExecuteATSilent("AT+QCCID", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseQCCID(resp), nil
}

func (m *Manager) QueryOperator() (string, error) {
	_, _ = m.ExecuteATSilent("AT+COPS=3,2", 2*time.Second)
	resp, err := m.ExecuteATSilent("AT+COPS?", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseCOPSOperator(resp), nil
}

func (m *Manager) QueryRegistration() (int, string, string, string, error) {
	resp, err := m.ExecuteATSilent("AT+CREG?", 2*time.Second)
	if err != nil {
		return 0, "", "", "", err
	}
	regStatus, lac, cellID, ok := parseCREG(resp)
	if !ok {
		return 0, "", "", "", nil
	}
	return regStatus, m.getRegStatusText(regStatus), lac, cellID, nil
}

func (m *Manager) QueryCSQ() (int, int, error) {
	resp, err := m.ExecuteATSilent("AT+CSQ", 2*time.Second)
	if err != nil {
		return 0, -999, err
	}
	rssi, dbm, ok := parseCSQ(resp)
	if !ok {
		return 0, -999, nil
	}
	return rssi, dbm, nil
}

func (m *Manager) QueryServingCellLTE() (int, int, error) {
	info, err := m.QueryServingCellLTEInfo()
	if err != nil {
		return 0, 0, err
	}
	return info.RSRP, info.RSRQ, nil
}

func (m *Manager) QueryServingCellLTEInfo() (ServingCellLTEInfo, error) {
	resp, err := m.ExecuteATSilent("AT+QENG=\"servingcell\"", 3*time.Second)
	if err != nil {
		return ServingCellLTEInfo{}, err
	}
	info, ok := parseServingCellLTEInfo(resp)
	if !ok {
		return ServingCellLTEInfo{}, nil
	}
	return info, nil
}

func (m *Manager) QueryAPN() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CGDCONT?", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseAPN(resp), nil
}

func (m *Manager) QueryIMSStatus() (int, error) {
	resp, err := m.ExecuteATSilent("AT+QIMS?", 2*time.Second)
	if err != nil {
		return 0, err
	}
	v, _ := parseQIMS(resp)
	return v, nil
}

func (m *Manager) QueryNetworkModeAndDuplex() (string, string, error) {
	mode, duplex, _, _, err := m.QueryNetworkRadio()
	return mode, duplex, err
}

func (m *Manager) QueryNetworkRadio() (string, string, string, uint32, error) {
	resp, err := m.ExecuteATSilent("AT+QNWINFO", 2*time.Second)
	if err != nil {
		return "", "", "", 0, err
	}
	mode, duplex, band, channel := parseQNWInfoRadio(resp)
	return mode, duplex, band, channel, nil
}

func (m *Manager) QueryNetworkMode() (string, error) {
	mode, _, err := m.QueryNetworkModeAndDuplex()
	return mode, err
}

func (m *Manager) QueryNetworkModeFallbackAndDuplex() (string, string, error) {
	networkMode, networkDuplex, err := m.QueryNetworkModeAndDuplex()
	if err == nil && networkMode != "" {
		return networkMode, networkDuplex, nil
	}
	resp, err2 := m.ExecuteATSilent("AT+COPS?", 2*time.Second)
	if err2 != nil {
		if err != nil {
			return "", "", err
		}
		return "", "", err2
	}
	mode, _ := parseCOPSAct(resp)
	return mode, "", nil
}

func (m *Manager) QueryNetworkModeFallback() (string, error) {
	mode, _, err := m.QueryNetworkModeFallbackAndDuplex()
	return mode, err
}

func (m *Manager) SetAttach(attached bool) error {
	cmd := "AT+CGATT=0"
	if attached {
		cmd = "AT+CGATT=1"
	}
	_, err := m.ExecuteATHigh(cmd, 5*time.Second)
	return err
}

func (m *Manager) SMSReadPDU(index string) (string, error) {
	resp, err := m.ExecuteAT("AT+CMGR="+index, 5*time.Second)
	if err != nil {
		return "", err
	}
	pdu, _ := extractSMSPDUAfterPrefix(resp, "+CMGR:")
	return pdu, nil
}

func (m *Manager) SMSListAllPDU() ([]string, error) {
	resp, err := m.ExecuteAT("AT+CMGL=4", 10*time.Second)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(resp) == "OK" {
		return nil, nil
	}
	return extractAllSMSPDUsAfterPrefix(resp, "+CMGL:"), nil
}

func (m *Manager) SMSDeleteAll() error {
	_, err := m.ExecuteAT("AT+CMGD=1,4", 5*time.Second)
	return err
}

// QuerySMSC 查询短信中心号码 (AT+CSCA?)
func (m *Manager) QuerySMSC() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CSCA?", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseCSCA(resp), nil
}

// QueryMSISDN 查询本机号码 (AT+CNUM)
func (m *Manager) QueryMSISDN() (string, error) {
	resp, err := m.ExecuteATSilent("AT+CNUM", 2*time.Second)
	if err != nil {
		return "", err
	}
	return parseCNUM(resp), nil
}

// QueryUSBNetMode 查询 USBNET 模式
func (m *Manager) QueryUSBNetMode() (int, error) {
	resp, err := m.ExecuteATSilent("AT+QCFG=\"usbnet\"?", 2*time.Second)
	if err != nil {
		return -1, err
	}
	mode, ok := parseUSBNet(resp)
	if !ok {
		return -1, nil
	}
	return mode, nil
}

// SetUSBNetMode 设置 USBNET 模式并重启
func (m *Manager) SetUSBNetMode(mode int) error {
	cmd := fmt.Sprintf("AT+QCFG=\"usbnet\",%d", mode)
	_, err := m.ExecuteAT(cmd, 5*time.Second)
	if err != nil {
		return fmt.Errorf("设置 USBNET 模式失败: %w", err)
	}

	// 重启模组以生效
	if _, err := m.ExecuteAT("AT+CFUN=1,1", 5*time.Second); err != nil {
		return fmt.Errorf("重启模组失败: %w", err)
	}

	return nil
}

// OpenLogicalChannel 通过 AT+CCHO 打开 eUICC 的 logical channel
func (m *Manager) OpenLogicalChannel(aid string) (int, error) {
	return m.openLogicalChannel(aid, "esim_session_open", "esim", apduarbiter.APDUClassEUICCWrite)
}

// OpenSIMAuthLogicalChannel 通过 AT+CCHO 打开 USIM/ISIM 鉴权 logical channel。
func (m *Manager) OpenSIMAuthLogicalChannel(aid string) (int, error) {
	return m.openLogicalChannel(aid, "sim_aka_open", "sim_aka", apduarbiter.APDUClassUSIMAKA)
}

func (m *Manager) ResolveSIMAuthAID(app string, fallbackAID string) (string, string, error) {
	fallback := simaid.NormalizeHexAID(fallbackAID)
	var match func([]byte) bool
	switch strings.ToLower(strings.TrimSpace(app)) {
	case "usim":
		match = simaid.IsUSIM
	case "isim":
		match = simaid.IsISIM
	default:
		return "", "sim_auth_aid_not_ready", fmt.Errorf("sim_auth_aid_not_ready: unsupported app %q", app)
	}
	aids, err := simaid.ReadDirectoryAIDs(func(apdu []byte) ([]byte, error) {
		respHex, err := m.TransmitBasicAPDU(hex.EncodeToString(apdu))
		if err != nil {
			return nil, err
		}
		return hex.DecodeString(respHex)
	})
	if err != nil {
		return "", "sim_auth_aid_not_ready", fmt.Errorf("sim_auth_aid_not_ready: %w", err)
	}
	for _, aid := range aids {
		if match(aid) {
			aidHex := strings.ToUpper(hex.EncodeToString(aid))
			if len(aidHex) <= len(fallback) {
				return "", "sim_auth_aid_not_ready", fmt.Errorf("sim_auth_aid_not_ready: %s AID is not full AID: %s", app, aidHex)
			}
			return aidHex, "at_ef_dir", nil
		}
	}
	return "", "sim_auth_aid_not_ready", fmt.Errorf("sim_auth_aid_not_ready: no %s full AID in EF_DIR", app)
}

func (m *Manager) openLogicalChannel(aid, leaseOwner, sessionOwner string, class apduarbiter.APDUClass) (int, error) {
	lease, err := m.acquireAPDUTransportLease(5*time.Second, leaseOwner, class, -1)
	if err != nil {
		return -1, err
	}
	if lease != nil {
		defer lease.Release()
		lease.Touch()
	}
	cmd := fmt.Sprintf("AT+CCHO=\"%s\"", aid)
	resp, err := m.ExecuteAT(cmd, 5*time.Second)
	if err != nil {
		return -1, fmt.Errorf("打开 logical channel 失败: %w", err)
	}
	channel, ok := parseCCHO(resp)
	if !ok {
		return -1, fmt.Errorf("解析 logical channel 响应失败: %s", resp)
	}
	if lease != nil {
		lease.Touch()
	}
	m.bindAPDUSession(channel, sessionOwner, class)
	return channel, nil
}

// TransmitAPDU 通过 AT+CGLA 在 logical channel 上透传 APDU
func (m *Manager) TransmitAPDU(channel int, apduHex string) (string, error) {
	owner := "esim_apdu"
	class := apduarbiter.APDUClassEUICCWrite
	if channel == 0 {
		owner = "basic_apdu"
		class = apduarbiter.APDUClassSMSC
	} else if session, ok := m.getAPDUSession(channel); ok {
		if session.Owner != "" {
			owner = session.Owner
		}
		if session.Class != "" {
			class = session.Class
		}
	} else {
		owner = "unbound_channel_apdu"
	}
	lease, err := m.acquireAPDUTransportLease(10*time.Second, owner, class, channel)
	if err != nil {
		return "", err
	}
	if lease != nil {
		defer lease.Release()
		lease.Touch()
	}

	cmd := fmt.Sprintf("AT+CGLA=%d,%d,\"%s\"", channel, len(apduHex), apduHex)
	resp, err := m.ExecuteATSilent(cmd, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("APDU 透传失败: %w", err)
	}
	apduResp, ok := parseCGLA(resp)
	if !ok {
		return "", fmt.Errorf("解析 APDU 响应失败: %s", resp)
	}
	if lease != nil {
		lease.Touch()
	}
	return apduResp, nil
}

// CloseLogicalChannel 通过 AT+CCHC 关闭 logical channel
func (m *Manager) CloseLogicalChannel(channel int) error {
	return m.closeLogicalChannel(channel, "esim_session_close", apduarbiter.APDUClassEUICCWrite)
}

// CloseSIMAuthLogicalChannel 通过 AT+CCHC 清理 USIM/ISIM 鉴权 logical channel。
func (m *Manager) CloseSIMAuthLogicalChannel(channel int) error {
	return m.closeLogicalChannel(channel, "sim_aka_close", apduarbiter.APDUClassRecovery)
}

func (m *Manager) closeLogicalChannel(channel int, defaultOwner string, defaultClass apduarbiter.APDUClass) error {
	session, ok := m.takeAPDUSession(channel)
	owner := defaultOwner
	class := defaultClass
	if ok {
		if session.Owner != "" {
			owner = session.Owner + "_close"
		}
		if session.Class == apduarbiter.APDUClassUSIMAKA {
			class = apduarbiter.APDUClassRecovery
		} else if session.Class != "" {
			class = session.Class
		}
	}
	lease, err := m.acquireAPDUTransportLease(5*time.Second, owner, class, channel)
	if err != nil {
		return err
	}
	if lease != nil {
		defer lease.Release()
		lease.Touch()
	}
	cmd := fmt.Sprintf("AT+CCHC=%d", channel)
	_, err = m.ExecuteAT(cmd, 5*time.Second)
	if err != nil {
		return fmt.Errorf("关闭 logical channel %d 失败: %w", channel, err)
	}
	if lease != nil {
		lease.Touch()
	}
	return nil
}

// ClearLogicalChannels 尝试关闭所有可能的逻辑通道 (1-4)
// 忽略执行结果，用于异常恢复和状态清理
func (m *Manager) ClearLogicalChannels() {
	for i := 1; i <= 4; i++ {
		m.takeAPDUSession(i)
		lease, err := m.acquireAPDUTransportLease(2*time.Second, "esim_session_clear", apduarbiter.APDUClassEUICCWrite, i)
		if err != nil {
			continue
		}
		if lease != nil {
			lease.Touch()
		}
		cmd := fmt.Sprintf("AT+CCHC=%d", i)
		m.ExecuteATSilent(cmd, 2*time.Second)
		if lease != nil {
			lease.Touch()
			lease.Release()
		}
	}
}

// TransmitBasicAPDU 通过 AT+CSIM 在基本通道（Channel 0）上发送 APDU。
func (m *Manager) TransmitBasicAPDU(apduHex string) (string, error) {
	lease, err := m.acquireAPDUTransportLease(8*time.Second, "sim_aka", apduarbiter.APDUClassUSIMAKA, 0)
	if err != nil {
		return "", err
	}
	if lease != nil {
		defer lease.Release()
		lease.Touch()
	}

	send := func(currentAPDUHex string) (string, error) {
		apduBytes, err := hex.DecodeString(currentAPDUHex)
		if err != nil {
			return "", fmt.Errorf("APDU hex 解码失败: %w", err)
		}
		try := func(length int) (string, bool, error) {
			cmd := fmt.Sprintf("AT+CSIM=%d,\"%s\"", length, currentAPDUHex)
			resp, err := m.ExecuteATSilent(cmd, 8*time.Second)
			if err != nil {
				return "", true, err
			}
			parsed, ok := parseCSIM(resp)
			if !ok {
				return "", false, fmt.Errorf("解析 CSIM 响应失败: %s", resp)
			}
			return parsed, false, nil
		}

		parsed, transportFailed, hexErr := try(len(currentAPDUHex))
		if hexErr == nil && !strings.EqualFold(parsed, "6700") {
			if lease != nil {
				lease.Touch()
			}
			return parsed, nil
		}
		if hexErr != nil && !transportFailed {
			return "", hexErr
		}

		parsed, _, byteErr := try(len(apduBytes))
		if byteErr == nil {
			if lease != nil {
				lease.Touch()
			}
			return parsed, nil
		}
		if hexErr != nil {
			return "", fmt.Errorf("CSIM 执行失败: hexlen_err=%v bytelen_err=%v", hexErr, byteErr)
		}
		return "", fmt.Errorf("CSIM 执行失败: hexlen_resp=%s bytelen_err=%v", parsed, byteErr)
	}

	parsed, err := send(apduHex)
	if err != nil {
		return "", err
	}
	return followUpBasicAPDU(send, apduHex, parsed, 3)
}

func followUpBasicAPDU(send func(string) (string, error), originalAPDUHex string, rspHex string, remaining int) (string, error) {
	rspHex = strings.TrimSpace(rspHex)
	rsp, err := hex.DecodeString(rspHex)
	if err != nil {
		return "", fmt.Errorf("APDU hex 解码失败: %w", err)
	}
	if len(rsp) < 2 || remaining <= 0 {
		return rspHex, nil
	}

	sw1 := rsp[len(rsp)-2]
	sw2 := rsp[len(rsp)-1]
	switch sw1 {
	case 0x61:
		nextAPDU := fmt.Sprintf("00c00000%02X", sw2)
		nextRsp, err := send(nextAPDU)
		if err != nil {
			return "", err
		}
		return followUpBasicAPDU(send, nextAPDU, nextRsp, remaining-1)
	case 0x6C:
		original, err := hex.DecodeString(originalAPDUHex)
		if err != nil {
			return "", fmt.Errorf("APDU hex 解码失败: %w", err)
		}
		if len(original) < 5 {
			return "", fmt.Errorf("APDU 收到 6C%02X 但原命令无 Le: %s", sw2, originalAPDUHex)
		}
		original[len(original)-1] = sw2
		nextAPDU := strings.ToLower(hex.EncodeToString(original))
		nextRsp, err := send(nextAPDU)
		if err != nil {
			return "", err
		}
		return followUpBasicAPDU(send, nextAPDU, nextRsp, remaining-1)
	default:
		return rspHex, nil
	}
}

// QueryNativeSPN 读取 SIM EF_SPN 服务提供商名称。
func (m *Manager) QueryNativeSPN() (string, error) {
	spnBytes, err := m.readSIMTransparentEF(28486, 17)
	if err != nil {
		return "", fmt.Errorf("read EF_SPN failed: %w", err)
	}
	return DecodeEFSPN(spnBytes)
}

func (m *Manager) readSIMTransparentEF(fileID int, length int) ([]byte, error) {
	resp, err := m.ExecuteATSilent(fmt.Sprintf("AT+CRSM=176,%d,0,0,%d", fileID, length), 2*time.Second)
	if err != nil {
		return nil, err
	}
	sw1, sw2, hexData, ok := ParseCRSM(resp)
	if !ok || sw1 != 144 || sw2 != 0 || len(hexData) == 0 {
		return nil, fmt.Errorf("invalid CRSM response for EF %d: %s (sw1=%d, sw2=%d)", fileID, resp, sw1, sw2)
	}
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, fmt.Errorf("EF %d hex decode failed: %w", fileID, err)
	}
	return data, nil
}

func (m *Manager) readSIMRecordEF(fileID int, record int, length int) ([]byte, error) {
	resp, err := m.ExecuteATSilent(fmt.Sprintf("AT+CRSM=178,%d,%d,4,%d", fileID, record, length), 2*time.Second)
	if err != nil {
		return nil, err
	}
	sw1, sw2, hexData, ok := ParseCRSM(resp)
	if !ok || sw1 != 144 || sw2 != 0 || len(hexData) == 0 {
		return nil, fmt.Errorf("invalid CRSM record response for EF %d record %d: %s (sw1=%d, sw2=%d)", fileID, record, resp, sw1, sw2)
	}
	data, err := hex.DecodeString(hexData)
	if err != nil {
		return nil, fmt.Errorf("EF %d record %d hex decode failed: %w", fileID, record, err)
	}
	return data, nil
}

func (m *Manager) QuerySIMMetadata() (*SIMMetadata, error) {
	meta := &SIMMetadata{}
	if data, err := m.readSIMTransparentEF(efGID1, 32); err == nil {
		meta.GID1 = simRawHex(data)
	}
	if data, err := m.readSIMTransparentEF(efGID2, 32); err == nil {
		meta.GID2 = simRawHex(data)
	}
	if st := m.querySIMServiceTable(); st != nil {
		meta.SIMServiceTable = st
	}
	meta.PNN = m.queryPNNRecords()
	meta.OPL = m.queryOPLRecords()
	if mcc, mnc, err := m.queryNativeMCCMNCFromIMSI(); err == nil {
		meta.NativeMCC = mcc
		meta.NativeMNC = mnc
	}
	if meta.NativeMCC == "" && meta.NativeMNC == "" && meta.GID1 == "" && meta.GID2 == "" && meta.SIMServiceTable == nil && len(meta.PNN) == 0 && len(meta.OPL) == 0 {
		return meta, fmt.Errorf("sim_metadata_empty")
	}
	return meta, nil
}

func (m *Manager) querySIMServiceTable() *SIMServiceTable {
	if data, err := m.readSIMTransparentEF(efUST, 32); err == nil {
		if st := DecodeSIMServiceTable("UST", data); st != nil {
			return st
		}
	}
	if data, err := m.readSIMTransparentEF(efSST, 16); err == nil {
		return DecodeSIMServiceTable("SST", data)
	}
	return nil
}

func (m *Manager) queryPNNRecords() []PNNRecord {
	records := make([]PNNRecord, 0)
	failures := 0
	for i := 1; i <= 32; i++ {
		data, err := m.readPNNRecord(i)
		if err != nil {
			failures++
			if failures >= 2 || len(records) > 0 {
				break
			}
			continue
		}
		failures = 0
		if rec, ok := DecodePNNRecord(i, data); ok {
			records = append(records, rec)
		}
	}
	return records
}

func (m *Manager) readPNNRecord(record int) ([]byte, error) {
	lengths := []int{32, 64, 16, 12}
	var lastErr error
	for _, length := range lengths {
		data, err := m.readSIMRecordEF(efPNN, record, length)
		if err != nil {
			lastErr = err
			continue
		}
		if pnnTLVLength(data) > 0 {
			return data, nil
		}
		lastErr = fmt.Errorf("PNN record %d has no valid TLV at length %d", record, length)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("PNN record %d read failed", record)
}

func (m *Manager) queryOPLRecords() []OPLRecord {
	records := make([]OPLRecord, 0)
	failures := 0
	for i := 1; i <= 32; i++ {
		data, err := m.readSIMRecordEF(efOPL, i, 8)
		if err != nil {
			failures++
			if failures >= 2 || len(records) > 0 {
				break
			}
			continue
		}
		failures = 0
		if rec, ok := DecodeOPLRecord(i, data); ok {
			records = append(records, rec)
		}
	}
	return records
}

func (m *Manager) QueryNativeMCCMNC() (mcc string, mnc string, err error) {
	return m.queryNativeMCCMNCFromIMSI()
}

func (m *Manager) queryNativeMCCMNCFromIMSI() (mcc string, mnc string, err error) {
	imsiStr, err := m.queryIMSIFromEF()
	if err != nil {
		return "", "", err
	}

	var adBytes []byte
	if data, errAD := m.readSIMTransparentEF(efAD, 4); errAD == nil {
		adBytes = data
	}

	mcc, mnc, _, _, err = HomeMCCMNCFromIMSIAndEFAD(imsiStr, adBytes)
	if err != nil {
		return "", "", err
	}
	return mcc, mnc, nil
}

func (m *Manager) queryIMSIFromEF() (string, error) {
	imsiBytes, err := m.readSIMTransparentEF(efIMSI, 9)
	if err != nil {
		return "", fmt.Errorf("read EF_IMSI failed: %w", err)
	}
	if len(imsiBytes) <= 1 {
		return "", fmt.Errorf("EF_IMSI data too short")
	}

	bcd := imsiBytes[1:]
	if int(imsiBytes[0]) <= len(imsiBytes)-1 {
		bcd = imsiBytes[1 : 1+int(imsiBytes[0])]
	}
	imsiStr := DecodeSwappedBCD(bcd)
	if len(imsiStr) > 0 {
		imsiStr = imsiStr[1:] // 移除 parity 位
	}

	if len(imsiStr) < 5 {
		return "", fmt.Errorf("decoded IMSI too short: %s", imsiStr)
	}

	return imsiStr, nil
}
