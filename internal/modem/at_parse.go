package modem

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WongLoki/DJ4Hub/pkg/smscodec"
)

func splitLines(resp string) []string {
	resp = strings.ReplaceAll(resp, "\r", "")
	out := strings.Split(resp, "\n")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func findLineWithPrefix(resp string, prefix string) (string, bool) {
	for _, line := range splitLines(resp) {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	return "", false
}

func extractFirstQuoted(resp string) (string, bool) {
	if i := strings.IndexByte(resp, '"'); i >= 0 {
		if j := strings.IndexByte(resp[i+1:], '"'); j >= 0 {
			return resp[i+1 : i+1+j], true
		}
	}
	return "", false
}

func extractQuotedFields(resp string) []string {
	fields := make([]string, 0, 4)
	s := resp
	for {
		i := strings.IndexByte(s, '"')
		if i < 0 {
			break
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '"')
		if j < 0 {
			break
		}
		fields = append(fields, s[:j])
		s = s[j+1:]
	}
	return fields
}

func parseIMEI(resp string) string {
	for _, line := range splitLines(resp) {
		if len(line) == 15 && strings.IndexFunc(line, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return line
		}
	}
	return ""
}

func parseFirmware(resp string) string {
	for _, line := range splitLines(resp) {
		if line != "" && line != "OK" && !strings.HasPrefix(line, "+") {
			return line
		}
	}
	return ""
}

func parseQSIMSTATInserted(resp string) (bool, bool) {
	line, ok := findLineWithPrefix(resp, "+QSIMSTAT:")
	if !ok {
		return false, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return false, false
	}
	var inserted int
	if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &inserted); err != nil {
		return false, false
	}
	return inserted == 1, true
}

func parseCPINInserted(resp string) (bool, bool) {
	s := strings.ToUpper(resp)
	if strings.Contains(s, "READY") {
		return true, true
	}
	if strings.Contains(s, "NOT INSERTED") {
		return false, true
	}
	return false, false
}

func parseIMSI(resp string) string {
	for _, line := range splitLines(resp) {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "+CIMI:") {
			s = strings.TrimSpace(strings.TrimPrefix(s, "+CIMI:"))
		}
		if len(s) >= 14 && len(s) <= 15 && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
			return s
		}
	}
	return ""
}

func parseQCCID(resp string) string {
	line, ok := findLineWithPrefix(resp, "+QCCID:")
	if !ok {
		return ""
	}
	if idx := strings.IndexByte(line, ':'); idx >= 0 {
		iccid := strings.TrimSpace(line[idx+1:])
		iccid = strings.Trim(iccid, "\"")
		iccid = strings.TrimRight(iccid, "Ff")
		return iccid
	}
	return ""
}

func parseCOPSOperator(resp string) string {
	line, ok := findLineWithPrefix(resp, "+COPS:")
	if !ok {
		return ""
	}
	fields := extractQuotedFields(line)
	if len(fields) < 1 {
		return ""
	}
	return ResolveServingOperatorNameFromPLMN(fields[0])
}

func parseCOPSAct(resp string) (string, bool) {
	line, ok := findLineWithPrefix(resp, "+COPS:")
	if !ok {
		line = strings.TrimSpace(resp)
	}
	if !strings.Contains(line, ",") {
		return "", false
	}
	lastComma := strings.LastIndex(line, ",")
	actStr := strings.TrimSpace(line[lastComma+1:])
	act := -1
	if _, err := fmt.Sscanf(actStr, "%d", &act); err != nil {
		return "", false
	}
	switch act {
	case 0:
		return "GSM", true
	case 2:
		return "WCDMA", true
	case 7:
		return "LTE", true
	case 10:
		return "NR", true
	default:
		return "", false
	}
}

func parseCREG(resp string) (int, string, string, bool) {
	line, ok := findLineWithPrefix(resp, "+CREG:")
	if !ok {
		return 0, "", "", false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return 0, "", "", false
	}
	statPart := strings.TrimSpace(parts[1])
	regStatus := 0
	if _, err := fmt.Sscanf(statPart, "%d", &regStatus); err != nil {
		return 0, "", "", false
	}
	lac := ""
	cellID := ""
	if len(parts) >= 4 {
		lac = strings.Trim(strings.TrimSpace(parts[2]), "\"")
		cellID = strings.Trim(strings.TrimSpace(parts[3]), "\"")
	}
	return regStatus, lac, cellID, true
}

func parseCSQ(resp string) (int, int, bool) {
	line, ok := findLineWithPrefix(resp, "+CSQ:")
	if !ok {
		return 0, -999, false
	}
	s := strings.TrimSpace(strings.TrimPrefix(line, "+CSQ:"))
	parts := strings.Split(s, ",")
	if len(parts) < 1 {
		return 0, -999, false
	}
	rssi := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &rssi); err != nil {
		return 0, -999, false
	}
	dbm := -999
	if rssi >= 0 && rssi <= 31 {
		dbm = -113 + (rssi * 2)
	}
	return rssi, dbm, true
}

type ServingCellLTEInfo struct {
	RSRP    int
	RSRQ    int
	SINR    int
	Duplex  string
	Band    string
	Channel uint32
}

func parseServingCellLTE(resp string) (int, int, bool) {
	info, ok := parseServingCellLTEInfo(resp)
	return info.RSRP, info.RSRQ, ok
}

func parseServingCellLTEInfo(resp string) (ServingCellLTEInfo, bool) {
	line, ok := findLineWithPrefix(resp, "+QENG:")
	if !ok {
		return ServingCellLTEInfo{}, false
	}
	if !strings.Contains(line, "LTE") {
		return ServingCellLTEInfo{}, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 10 {
		return ServingCellLTEInfo{}, false
	}

	info := ServingCellLTEInfo{}
	if len(parts) > 3 {
		info.Duplex = strings.Trim(strings.TrimSpace(parts[3]), "\"")
	}
	if channel, err := strconv.ParseUint(strings.TrimSpace(parts[8]), 10, 32); err == nil {
		info.Channel = uint32(channel)
	}
	band := strings.TrimSpace(parts[9])
	if band != "" {
		info.Band = "LTE BAND " + strings.Trim(band, "\"")
	}

	tail := parts[len(parts)-5:]
	parsed := make([]int, 0, len(tail))
	for _, part := range tail {
		part = strings.TrimSpace(part)
		var val int
		if _, err := fmt.Sscanf(part, "%d", &val); err != nil {
			return ServingCellLTEInfo{}, false
		}
		parsed = append(parsed, val)
	}
	if len(parsed) != 5 {
		return ServingCellLTEInfo{}, false
	}

	info.RSRP = parsed[0]
	info.RSRQ = parsed[1]
	info.SINR = parsed[3]
	if info.RSRP < -140 || info.RSRP > -40 {
		return ServingCellLTEInfo{}, false
	}
	if info.RSRQ < -30 || info.RSRQ > 0 {
		return ServingCellLTEInfo{}, false
	}
	return info, true
}

func parseAPN(resp string) string {
	for _, line := range splitLines(resp) {
		if !strings.HasPrefix(line, "+CGDCONT:") {
			continue
		}
		fields := extractQuotedFields(line)
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	return ""
}

func parseQIMS(resp string) (int, bool) {
	line, ok := findLineWithPrefix(resp, "+QIMS:")
	if !ok {
		return 0, false
	}
	if idx := strings.IndexByte(line, ':'); idx >= 0 {
		v := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(line[idx+1:]), "%d", &v); err == nil {
			return v, true
		}
	}
	return 0, false
}

func parseQNWINFO(resp string) string {
	mode, duplex := parseQNWInfoModeAndDuplex(resp)
	if mode == "" {
		return ""
	}
	if duplex != "" {
		return duplex + " " + mode
	}
	return mode
}

func parseQNWInfoModeAndDuplex(resp string) (string, string) {
	mode, duplex, _, _ := parseQNWInfoRadio(resp)
	return mode, duplex
}

func parseQNWInfoRadio(resp string) (string, string, string, uint32) {
	line, ok := findLineWithPrefix(resp, "+QNWINFO:")
	if !ok {
		return "", "", "", 0
	}
	fields := extractQuotedFields(line)
	if len(fields) < 1 {
		return "", "", "", 0
	}
	mode := strings.TrimSpace(fields[0])
	duplex := ""
	switch mode {
	case "FDD LTE":
		mode = "LTE"
		duplex = "FDD"
	case "TDD LTE":
		mode = "LTE"
		duplex = "TDD"
	}

	band := ""
	if len(fields) >= 3 {
		band = strings.TrimSpace(fields[2])
	}

	channel := uint32(0)
	if idx := strings.LastIndex(line, ","); idx >= 0 && idx+1 < len(line) {
		v := strings.TrimSpace(line[idx+1:])
		if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
			channel = uint32(parsed)
		}
	}
	return mode, duplex, band, channel
}

func extractNextLineAfterPrefix(resp string, prefix string) (string, bool) {
	lines := splitLines(resp)
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], prefix) {
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && next != "OK" {
					return next, true
				}
			}
			return "", false
		}
	}
	return "", false
}

func extractAllNextLinesAfterPrefix(resp string, prefix string) []string {
	lines := splitLines(resp)
	out := make([]string, 0)
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], prefix) {
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && next != "OK" {
					out = append(out, next)
				}
				i++
			}
		}
	}
	return out
}

func extractSMSPDUAfterPrefix(resp string, prefix string) (string, bool) {
	lines := splitLines(resp)
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], prefix) {
			if i+1 >= len(lines) {
				return "", false
			}
			next := strings.TrimSpace(lines[i+1])
			if next == "" || next == "OK" {
				return "", false
			}
			pdu, _ := smscodec.TrimFullPDUHexByATHeader(next, lines[i])
			return pdu, true
		}
	}
	return "", false
}

func extractAllSMSPDUsAfterPrefix(resp string, prefix string) []string {
	lines := splitLines(resp)
	out := make([]string, 0)
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], prefix) {
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if next != "" && next != "OK" {
					pdu, _ := smscodec.TrimFullPDUHexByATHeader(next, lines[i])
					out = append(out, pdu)
				}
				i++
			}
		}
	}
	return out
}

// parseCSCA 解析 AT+CSCA? 响应，提取短信中心号码
// 响应格式: +CSCA: "+447870002308",145
func parseCSCA(resp string) string {
	line, ok := findLineWithPrefix(resp, "+CSCA:")
	if !ok {
		return ""
	}
	// 提取第一个引号内的内容（短信中心号码）
	v, ok := extractFirstQuoted(line)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

// parseCNUM 解析 AT+CNUM 响应，提取第一个有效本机号码。
// 响应格式: +CNUM: "label","+8613800138000",145
func parseCNUM(resp string) string {
	for _, line := range splitLines(resp) {
		if !strings.HasPrefix(line, "+CNUM:") {
			continue
		}
		fields := extractQuotedFields(line)
		for _, field := range fields {
			candidate := canonicalPhoneCandidate(field)
			if candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

// ParseMSISDNResponse 解析 AT+CNUM 响应并返回第一个有效本机号码。
// internal/modem 之外的 USB AT 适配层使用此入口保持号码校验一致。
func ParseMSISDNResponse(resp string) string {
	return parseCNUM(resp)
}

func canonicalPhoneCandidate(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return ""
	}
	upper := strings.ToUpper(s)
	if upper == "FFFFFFFF" || upper == "00000000000" {
		return ""
	}
	if strings.EqualFold(s, "Own Number") {
		return ""
	}

	s = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(s), "tel:"))
	if strings.HasPrefix(strings.ToLower(v), "tel:") {
		s = strings.TrimSpace(v[4:])
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	if s == "" {
		return ""
	}
	digits := s
	if strings.HasPrefix(digits, "+") {
		digits = digits[1:]
	}
	if len(digits) < 6 {
		return ""
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return ""
		}
	}
	return s
}

// parseUSBNet 解析 AT+QCFG="usbnet" 响应
// 响应格式: +QCFG: "usbnet",0
func parseUSBNet(resp string) (int, bool) {
	line, ok := findLineWithPrefix(resp, "+QCFG:")
	if !ok {
		return -1, false
	}
	if !strings.Contains(line, "\"usbnet\"") {
		return -1, false
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return -1, false
	}
	modePart := strings.TrimSpace(parts[1])
	mode := -1
	if _, err := fmt.Sscanf(modePart, "%d", &mode); err == nil {
		return mode, true
	}
	return -1, false
}

// parseCCHO 解析 AT+CCHO 响应，提取逻辑通道号
// 响应格式: +CCHO: <channel>
func parseCCHO(resp string) (int, bool) {
	line, ok := findLineWithPrefix(resp, "+CCHO:")
	if !ok {
		return -1, false
	}
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return -1, false
	}
	channelPart := strings.TrimSpace(parts[1])
	channel := -1
	if _, err := fmt.Sscanf(channelPart, "%d", &channel); err == nil {
		return channel, true
	}
	return -1, false
}

// parseCGLA 解析 AT+CGLA 响应，提取 APDU 响应
// 响应格式: +CGLA: <len>,"<apdu>"
func parseCGLA(resp string) (string, bool) {
	line, ok := findLineWithPrefix(resp, "+CGLA:")
	if !ok {
		return "", false
	}
	apduStr, ok := extractFirstQuoted(line)
	if !ok {
		return "", false
	}
	return apduStr, true
}

// parseCSIM 解析 AT+CSIM 响应，提取 APDU 响应
// 响应格式: +CSIM: <len>,"<apdu>"
func parseCSIM(resp string) (string, bool) {
	line, ok := findLineWithPrefix(resp, "+CSIM:")
	if !ok {
		return "", false
	}
	apduStr, ok := extractFirstQuoted(line)
	if !ok {
		return "", false
	}
	return apduStr, true
}

// ParseCRSM 解析 AT+CRSM 响应
// 返回值格式为：sw1, sw2, hex_data, ok
// 以 +CRSM: 144,0,"082984021385481729" 为例 -> sw1=144, sw2=0, data="082984021385481729"
func ParseCRSM(resp string) (int, int, string, bool) {
	line, ok := findLineWithPrefix(resp, "+CRSM:")
	if !ok {
		return 0, 0, "", false
	}
	// +CRSM: <sw1>,<sw2>,"<response>"
	s := strings.TrimSpace(strings.TrimPrefix(line, "+CRSM:"))
	parts := strings.SplitN(s, ",", 3)
	if len(parts) < 2 {
		return 0, 0, "", false
	}

	sw1, sw2 := 0, 0
	fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &sw1)
	fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &sw2)

	data := ""
	if len(parts) == 3 {
		data = strings.TrimSpace(parts[2])
		data = strings.Trim(data, "\"")
	}
	return sw1, sw2, data, true
}

func DecodeSwappedBCD(data []byte) string {
	out := make([]byte, 0, len(data)*2)
	for _, b := range data {
		low := b & 0x0F
		high := (b >> 4) & 0x0F

		if low <= 9 {
			out = append(out, '0'+byte(low))
		}
		if high <= 9 {
			out = append(out, '0'+byte(high))
		}
	}
	return string(out)
}

func parseCOPSScan(resp string) []OperatorScanEntry {
	var entries []OperatorScanEntry
	line, ok := findLineWithPrefix(resp, "+COPS: ")
	if !ok {
		return entries
	}

	// Example format: +COPS: (2,"CHN-UNICOM","UNICOM","46001",7),(3,"CHINA MOBILE","CMCC","46000",7),,(0,1,2,3,4),(0,1,2)
	// We extract everything between parentheses using simple parsing
	parts := strings.Split(line[7:], "),(")
	for _, part := range parts {
		part = strings.TrimPrefix(part, "(")
		part = strings.TrimSuffix(part, ")")

		// The list ends with supported modes/formats, usually just numbers. We can skip if it doesn't match operator format.
		// Usually: (stat,long,short,numeric,Act)
		fields := strings.Split(part, ",")
		if len(fields) >= 4 {
			stat, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}
			longName := strings.Trim(fields[1], "\"")
			shortName := strings.Trim(fields[2], "\"")
			plmn := strings.Trim(fields[3], "\"")

			act := 0
			if len(fields) >= 5 {
				act, _ = strconv.Atoi(fields[4])
			}

			if plmn == "" {
				continue
			}

			entries = append(entries, OperatorScanEntry{
				Status:    stat,
				LongName:  longName,
				ShortName: shortName,
				PLMN:      plmn,
				Act:       act,
			})
		}
	}
	return entries
}

func parseCOPSSelection(resp string) (OperatorSelectionState, bool) {
	line, ok := findLineWithPrefix(resp, "+COPS: ")
	if !ok {
		return OperatorSelectionState{}, false
	}

	// Example: +COPS: 1,2,"46001",7 or +COPS: 0
	fields := strings.Split(line[7:], ",")
	if len(fields) == 0 {
		return OperatorSelectionState{}, false
	}

	mode, err := strconv.Atoi(fields[0])
	if err != nil {
		return OperatorSelectionState{}, false
	}

	state := OperatorSelectionState{
		Mode: mode,
	}

	if len(fields) >= 3 {
		format, err := strconv.Atoi(fields[1])
		if err == nil {
			state.Format = format
		}
		state.PLMN = strings.Trim(fields[2], "\"")
	}

	if len(fields) >= 4 {
		act, err := strconv.Atoi(fields[3])
		if err == nil {
			state.Act = act
			state.HasAct = true
		}
	}

	return state, true
}
