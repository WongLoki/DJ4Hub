package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/WongLoki/dj-4g-hub/internal/modem"
	"github.com/WongLoki/dj-4g-hub/pkg/smscodec"
)

// ATBackend AT 后端适配器 — 纯包装层，委托给现有 modem.Manager
// 不修改 modem.Manager 的任何一行代码
type ATBackend struct {
	modem *modem.Manager
}

// NewATBackend 创建 AT 后端适配器
func NewATBackend(m *modem.Manager) *ATBackend {
	return &ATBackend{modem: m}
}

// Mode 返回后端模式标识
func (a *ATBackend) Mode() string { return "at" }

// Close AT 后端无需额外清理（modem.Manager 由 Worker 管理生命周期）
func (a *ATBackend) Close() error { return nil }

// Modem 返回底层 modem.Manager（供需要直接访问 AT 通道的调用方使用，如 AT+QCFG）
func (a *ATBackend) Modem() *modem.Manager { return a.modem }

// ============================================================================
// DeviceInfoProvider 实现
// ============================================================================

func (a *ATBackend) GetIMEI(ctx context.Context) (string, error) {
	return a.modem.QueryIMEI()
}

func (a *ATBackend) GetIMSI(ctx context.Context) (string, error) {
	return a.modem.QueryIMSI()
}

// GetIMSILive AT 模式下 IMSI 本身即实时读取。
func (a *ATBackend) GetIMSILive(ctx context.Context) (string, error) {
	return a.modem.QueryIMSI()
}

func (a *ATBackend) GetICCID(ctx context.Context) (string, error) {
	return a.modem.QueryICCID()
}

func (a *ATBackend) GetMSISDN(ctx context.Context) (string, error) {
	return a.modem.QueryMSISDN()
}

// GetICCIDLive AT 模式下 ICCID 本身即实时读取。
func (a *ATBackend) GetICCIDLive(ctx context.Context) (string, error) {
	return a.modem.QueryICCID()
}

func (a *ATBackend) GetRevision(ctx context.Context) (string, error) {
	return a.modem.QueryFirmware()
}

func (a *ATBackend) GetSignalInfo(ctx context.Context) (*SignalInfo, error) {
	info := &SignalInfo{}

	// AT+CSQ → RSSI/dBm
	if _, dbm, err := a.modem.QueryCSQ(); err == nil {
		info.RSSI = dbm
	}

	// AT+QENG="servingcell" → RSRP/RSRQ/SINR
	if cell, err := a.modem.QueryServingCellLTEInfo(); err == nil {
		info.RSRP = cell.RSRP
		info.RSRQ = cell.RSRQ
		info.SINR = cell.SINR
	}

	return info, nil
}

func (a *ATBackend) GetServingSystem(ctx context.Context) (*ServingSystem, error) {
	ss := &ServingSystem{}

	// AT+CREG? → 注册状态、LAC、CellID
	if regStatus, regText, lac, cellID, err := a.modem.QueryRegistration(); err == nil {
		ss.RegStatus = regStatus
		ss.RegStatusText = regText
		ss.LAC = lac
		ss.CellID = cellID
	}

	// AT+COPS? → 运营商
	if operator, err := a.modem.QueryOperator(); err == nil {
		ss.Operator = operator
	}

	// AT+QNWINFO → 网络模式 / 双工方式 / 频段 / 信道
	if mode, duplex, band, channel, err := a.modem.QueryNetworkRadio(); err == nil {
		ss.NetworkMode = mode
		ss.NetworkDuplex = duplex
		ss.RadioBand = band
		ss.RadioChannel = channel
	}

	return ss, nil
}

func (a *ATBackend) IsSimInserted(ctx context.Context) (bool, error) {
	return a.modem.QuerySIMInserted()
}

func (a *ATBackend) GetNativeMCCMNC(ctx context.Context) (mcc, mnc string, err error) {
	return a.modem.QueryNativeMCCMNC()
}

func (a *ATBackend) GetNativeSPN(ctx context.Context) (string, error) {
	return a.modem.QueryNativeSPN()
}

func (a *ATBackend) GetNativeSPNLive(ctx context.Context) (string, error) {
	return a.modem.QueryNativeSPN()
}

func (a *ATBackend) GetSIMMetadata(ctx context.Context) (*SIMMetadata, error) {
	meta, err := a.modem.QuerySIMMetadata()
	return mapModemSIMMetadata(meta), err
}

func (a *ATBackend) GetSIMMetadataLive(ctx context.Context) (*SIMMetadata, error) {
	return a.GetSIMMetadata(ctx)
}

func mapModemSIMMetadata(meta *modem.SIMMetadata) *SIMMetadata {
	if meta == nil {
		return nil
	}
	out := &SIMMetadata{
		NativeMCC: meta.NativeMCC,
		NativeMNC: meta.NativeMNC,
		GID1:      meta.GID1,
		GID2:      meta.GID2,
	}
	if len(meta.PNN) > 0 {
		out.PNN = make([]PNNRecord, 0, len(meta.PNN))
		for _, rec := range meta.PNN {
			out.PNN = append(out.PNN, PNNRecord(rec))
		}
	}
	if len(meta.OPL) > 0 {
		out.OPL = make([]OPLRecord, 0, len(meta.OPL))
		for _, rec := range meta.OPL {
			out.OPL = append(out.OPL, OPLRecord(rec))
		}
	}
	if meta.SIMServiceTable != nil {
		out.ServiceTable = (*SIMServiceTable)(meta.SIMServiceTable)
	}
	return out
}

// GetSMSC 读取短信中心号码（AT+CSCA?）。
func (a *ATBackend) GetSMSC(ctx context.Context) (string, error) {
	return a.modem.QuerySMSC()
}

// ============================================================================
// SMSProvider 实现
// ============================================================================

func (a *ATBackend) SendSMS(ctx context.Context, to, body string) error {
	return a.SendSMSWithOptions(ctx, to, body, smscodec.SubmitOptions{})
}

func (a *ATBackend) SendSMSWithOptions(ctx context.Context, to, body string, opts smscodec.SubmitOptions) error {
	return a.modem.SendSMSWithOptions(to, body, opts)
}

func (a *ATBackend) ReadSMS(ctx context.Context, index int) (*SMS, error) {
	// 委托给 modem 读取 PDU 并解码
	pdu, err := a.modem.SMSReadPDU(fmt.Sprintf("%d", index))
	if err != nil {
		return nil, err
	}
	if pdu == "" {
		return nil, fmt.Errorf("短信 %d 不存在或为空", index)
	}
	// 返回原始 PDU 数据; 完整解码由上层处理
	return &SMS{
		Index:   index,
		Content: pdu, // PDU 原文
	}, nil
}

func (a *ATBackend) DeleteSMS(ctx context.Context, index int) error {
	cmd := fmt.Sprintf("AT+CMGD=%d", index)
	_, err := a.modem.ExecuteAT(cmd, 5*time.Second)
	return err
}

func (a *ATBackend) ListSMS(ctx context.Context) ([]SMSSummary, error) {
	pdus, err := a.modem.SMSListAllPDU()
	if err != nil {
		return nil, err
	}
	// 返回 PDU 数量的概要；实际索引需解析 +CMGL 响应
	result := make([]SMSSummary, 0, len(pdus))
	for i := range pdus {
		result = append(result, SMSSummary{Index: i})
	}
	return result, nil
}

func (a *ATBackend) DeleteAllSMS(ctx context.Context) error {
	return a.modem.SMSDeleteAll()
}

// ============================================================================
// USSDProvider 实现
// ============================================================================

func (a *ATBackend) ExecuteUSSD(ctx context.Context, command string, timeout time.Duration) (*USSDResult, error) {
	result, err := a.modem.ExecuteUSSD(command, timeout)
	if err != nil {
		return nil, err
	}
	return modemUSSDResult(result), nil
}

func (a *ATBackend) CancelUSSD(ctx context.Context) error {
	a.modem.CancelUSSD()
	return nil
}

// ============================================================================
// OperatingModeController 实现
// ============================================================================

func (a *ATBackend) SetOperatingMode(ctx context.Context, mode OperatingMode) error {
	cmd := fmt.Sprintf("AT+CFUN=%d", int(mode))
	_, err := a.modem.ExecuteAT(cmd, 5*time.Second)
	return err
}

func (a *ATBackend) GetOperatingMode(ctx context.Context) (OperatingMode, error) {
	resp, err := a.modem.ExecuteATSilent("AT+CFUN?", 2*time.Second)
	if err != nil {
		return ModeOnline, err
	}
	// 解析 +CFUN: N
	var mode int
	if _, err := fmt.Sscanf(resp, "+CFUN: %d", &mode); err != nil {
		return ModeOnline, fmt.Errorf("解析 CFUN 响应失败: %s", resp)
	}
	return OperatingMode(mode), nil
}

func (a *ATBackend) Reboot(ctx context.Context) error {
	_, err := a.modem.ExecuteAT("AT+CFUN=1,1", 5*time.Second)
	return err
}

// ============================================================================
// SIMAuthProvider 实现
// ============================================================================

func (a *ATBackend) OpenLogicalChannel(ctx context.Context, aid string) (int, error) {
	return a.modem.OpenSIMAuthLogicalChannel(aid)
}

func (a *ATBackend) ResolveSIMAuthAID(ctx context.Context, app string, fallbackAID string) (string, string, error) {
	return a.modem.ResolveSIMAuthAID(app, fallbackAID)
}

func (a *ATBackend) CloseLogicalChannel(ctx context.Context, channelID int) error {
	return a.modem.CloseSIMAuthLogicalChannel(channelID)
}

func (a *ATBackend) TransmitAPDU(ctx context.Context, channelID int, command string) (string, error) {
	return a.modem.TransmitAPDU(channelID, command)
}
