package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type voiceCall struct {
	Direction int    `json:"direction"`
	ID        int    `json:"id"`
	State     int    `json:"state"`
	Number    string `json:"number"`
}

func parseVoiceCalls(raw string) []voiceCall {
	calls := []voiceCall{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+CLCC:") {
			continue
		}
		reader := csv.NewReader(strings.NewReader(strings.TrimSpace(strings.TrimPrefix(line, "+CLCC:"))))
		reader.TrimLeadingSpace = true
		fields, err := reader.Read()
		if err != nil || len(fields) < 5 || fields[3] != "0" {
			continue
		}
		id, e1 := strconv.Atoi(fields[0])
		state, e2 := strconv.Atoi(fields[2])
		if e1 != nil || e2 != nil {
			continue
		}
		call := voiceCall{ID: id, State: state}
		call.Direction, _ = strconv.Atoi(fields[1])
		if len(fields) > 5 {
			call.Number = fields[5]
		}
		calls = append(calls, call)
	}
	return calls
}

func (a *app) phoneCommand(command string) (string, error) {
	raw, err := a.runATCommand(command, 8*time.Second)
	if err != nil {
		return raw, err
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "ERROR" || strings.HasPrefix(line, "+CME ERROR:") || line == "NO CARRIER" || line == "BUSY" {
			return raw, fmt.Errorf("模块拒绝操作：%s", line)
		}
	}
	return raw, nil
}

func (a *app) callStatus(w http.ResponseWriter, r *http.Request) {
	raw, err := a.phoneCommand("AT+CLCC")
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"calls": parseVoiceCalls(raw)})
}

func (a *app) callAction(w http.ResponseWriter, r *http.Request) {
	// Serialize dialing with USB audio preparation/re-enumeration.
	a.audioMu.Lock()
	defer a.audioMu.Unlock()
	var body struct {
		Action string `json:"action"`
		Number string `json:"number"`
		Digit  string `json:"digit"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	command := ""
	switch body.Action {
	case "dial":
		if !regexp.MustCompile(`^\+?[0-9]{1,20}$`).MatchString(body.Number) {
			writeError(w, 400, "请输入有效电话号码")
			return
		}
		command = "ATD" + body.Number + ";"
	case "answer":
		command = "ATA"
	case "hangup":
		command = "AT+CHUP"
	case "dtmf":
		if !regexp.MustCompile(`^[0-9*#A-D]$`).MatchString(body.Digit) {
			writeError(w, 400, "无效按键")
			return
		}
		command = `AT+VTS="` + body.Digit + `"`
	default:
		writeError(w, 400, "未知电话操作")
		return
	}
	identity := ""
	if body.Action != "dtmf" {
		identity = a.historyIdentity()
	}
	if body.Action == "hangup" {
		a.captureCallHistory(identity)
	}
	if _, err := a.phoneCommand(command); err != nil {
		if body.Action == "dial" {
			now := time.Now()
			_ = a.appendHistory(historyRecord{Kind: "call", ICCID: confirmedHistoryIdentity(identity, a.historyIdentity()), Direction: "outgoing", Number: body.Number, State: "failed", Started: now, Ended: &now})
		}
		writeError(w, 502, err.Error())
		return
	}
	if body.Action != "dtmf" {
		a.captureCallHistory(identity)
	}
	writeJSON(w, 200, map[string]bool{"accepted": true})
}

func (a *app) captureCallHistory(identity string) {
	h := a.historyStore()
	if h == nil {
		return
	}
	raw, err := a.phoneCommand("AT+CLCC")
	if err == nil {
		_ = h.observe(parseVoiceCalls(raw), confirmedHistoryIdentity(identity, a.historyIdentity()), time.Now())
	}
}

func (a *app) saveAPN(w http.ResponseWriter, r *http.Request) {
	var body struct {
		APN string `json:"apn"`
		PDN string `json:"pdn"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !regexp.MustCompile(`^[A-Za-z0-9.-]{1,100}$`).MatchString(body.APN) || (body.PDN != "IP" && body.PDN != "IPV4V6" && body.PDN != "IPV6") {
		writeError(w, 400, "APN 或 IP 类型无效")
		return
	}
	raw, err := a.phoneCommand("AT+CLCC")
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	if len(parseVoiceCalls(raw)) > 0 {
		writeError(w, 409, "请在通话结束后修改 APN")
		return
	}
	if _, err = a.phoneCommand(fmt.Sprintf(`AT+CGDCONT=1,"%s","%s"`, body.PDN, body.APN)); err != nil {
		writeError(w, 502, err.Error())
		return
	}
	raw, err = a.phoneCommand("AT+CGDCONT?")
	if err != nil {
		writeError(w, 502, "写入后无法确认 APN，请刷新检查")
		return
	}
	for _, ctx := range parsePDPContexts(raw) {
		if ctx.ID == 1 && ctx.APN == body.APN && ctx.PDN == body.PDN {
			writeJSON(w, 200, map[string]string{"summary": "APN 已保存，下次数据连接时生效"})
			return
		}
	}
	writeError(w, 502, "模块未返回预期 APN，请刷新检查")
}
