package backend

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/WongLoki/dj-4g-hub/internal/modem"
	"github.com/WongLoki/dj-4g-hub/pkg/mbim"
	"github.com/iniwex5/quectel-qmi-go/pkg/manager"
)

// MBIMBackend implements DeviceBackend over an MBIM modem.
type MBIMBackend struct {
	source      MBIMSource
	controlPath string
}

var _ DeviceBackend = (*MBIMBackend)(nil)

func NewMBIMBackend(controlPath string, source MBIMSource) *MBIMBackend {
	return &MBIMBackend{source: source, controlPath: controlPath}
}

func (b *MBIMBackend) Mode() string { return BackendMBIM }
func (b *MBIMBackend) Close() error { return nil }

func (b *MBIMBackend) GetIMEI(ctx context.Context) (string, error) {
	caps, err := b.source.DeviceCaps(ctx)
	if err != nil {
		return "", err
	}
	return caps.DeviceID, nil
}

func (b *MBIMBackend) GetIMSI(ctx context.Context) (string, error) {
	sub, err := b.source.SubscriberReady(ctx)
	if err != nil {
		return "", err
	}
	return sub.IMSI, nil
}

func (b *MBIMBackend) GetICCID(ctx context.Context) (string, error) {
	sub, err := b.source.SubscriberReady(ctx)
	if err != nil {
		return "", err
	}
	return sub.ICCID, nil
}

func (b *MBIMBackend) GetMSISDN(ctx context.Context) (string, error) {
	sub, err := b.source.SubscriberReady(ctx)
	if err != nil {
		return "", err
	}
	if sub.MSISDN != "" {
		return sub.MSISDN, nil
	}
	return b.readMSISDNFromEF(ctx), nil
}

func (b *MBIMBackend) GetRevision(ctx context.Context) (string, error) {
	caps, err := b.source.DeviceCaps(ctx)
	if err != nil {
		return "", err
	}
	return caps.FirmwareInfo, nil
}

func (b *MBIMBackend) IsSimInserted(ctx context.Context) (bool, error) {
	sub, err := b.source.SubscriberReady(ctx)
	if err != nil {
		return false, err
	}
	return sub.ReadyState != 2, nil
}

func (b *MBIMBackend) GetSignalInfo(ctx context.Context) (*SignalInfo, error) {
	s, err := b.source.SignalState(ctx)
	if err != nil {
		return nil, err
	}
	info := &SignalInfo{}
	if !s.Unknown {
		info.RSSI = s.DBM
	}
	// MBIMEx 2.0 信号:RSRP/SINR(SNR)。RSRQ 在 MBIM(Ex) 协议中无对应字段,保持空。
	if s.HasRSRP {
		info.RSRP = s.RSRP
	}
	if s.HasSNR {
		info.SINR = s.SNR
	}
	return info, nil
}

func (b *MBIMBackend) GetServingSystem(ctx context.Context) (*ServingSystem, error) {
	rs, err := b.source.RegisterState(ctx)
	if err != nil {
		return nil, err
	}
	ss := &ServingSystem{
		Operator: mbimOperatorDisplay(rs.ProviderName, rs.MCC, rs.MNC),
		MCC:      atou16(rs.MCC),
		MNC:      atou16(rs.MNC),
	}
	ss.RegStatus, ss.RegStatusText = mapMBIMRegisterState(rs.RegisterState)
	if ps, err := b.source.PacketService(ctx); err == nil {
		ss.PSAttached = ps.State == 2
		ss.NetworkMode = mbimDataClassToNetworkMode(ps.HighestClass)
	}
	return ss, nil
}

func mbimOperatorDisplay(name, mcc, mnc string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	if len(mcc) != 3 || (len(mnc) != 2 && len(mnc) != 3) {
		return ""
	}
	plmn := mcc + mnc
	if display, ok := modem.LookupServingOperatorNameFromPLMN(plmn); ok {
		return display
	}
	return plmn
}

// mbimDataClassToNetworkMode 把 MBIM_DATA_CLASS 位掩码映射为面板用的接入技术字符串,
// 取已就绪的最高制式。频段/信道(RadioBand/RadioChannel)在标准 MBIM/MBIMEx 里
// 没有承载 CID,故此处不填,保持空。
func mbimDataClassToNetworkMode(dataClass uint32) string {
	const (
		dcGPRS  = 0x00000001
		dcEDGE  = 0x00000002
		dcUMTS  = 0x00000004
		dcHSDPA = 0x00000008
		dcHSUPA = 0x00000010
		dcLTE   = 0x00000020
		dc5GNSA = 0x00000040
		dc5GSA  = 0x00000080
	)
	switch {
	case dataClass&(dc5GNSA|dc5GSA) != 0:
		return "NR5G"
	case dataClass&dcLTE != 0:
		return "LTE"
	case dataClass&(dcUMTS|dcHSDPA|dcHSUPA) != 0:
		return "UMTS"
	case dataClass&(dcGPRS|dcEDGE) != 0:
		return "GSM"
	default:
		return ""
	}
}

func (b *MBIMBackend) GetNativeMCCMNC(ctx context.Context) (string, string, error) {
	// 优先用 HomeProvider 的 PLMN:模组按正确的 MNC 长度给出(5 或 6 位)。
	if hp, err := b.source.HomeProvider(ctx); err == nil {
		if mcc, mnc, ok := splitMBIMPLMN(hp.PLMN); ok {
			return mcc, mnc, nil
		}
	}
	// 兜底:与 QMI 同一套规则——IMSI + EF_AD 的 MNC 长度;EF_AD 读不到(如此固件
	// USIM ADF 逻辑通道开不了)时,HomeMCCMNCFromIMSIAndEFAD 会用 IMSI+MCC 表
	// 推断 MNC 长度(如 310→3 位),避免把 3 位 MNC 截成 2 位。
	sub, err := b.source.SubscriberReady(ctx)
	if err != nil {
		return "", "", err
	}
	efAD, _ := b.source.ReadSIMEF(ctx, efAD, 0)
	mcc, mnc, _, _, err := modem.HomeMCCMNCFromIMSIAndEFAD(sub.IMSI, efAD)
	if err != nil {
		return "", "", err
	}
	return mcc, mnc, nil
}

// splitMBIMPLMN 把 MBIM ProviderId(PLMN 字符串)拆成 MCC + 变长 MNC。
func splitMBIMPLMN(plmn string) (mcc, mnc string, ok bool) {
	if len(plmn) < 5 || len(plmn) > 6 {
		return "", "", false
	}
	return plmn[:3], plmn[3:], true
}

func (b *MBIMBackend) GetNativeSPN(ctx context.Context) (string, error) {
	return b.readNativeSPN(ctx)
}

func (b *MBIMBackend) GetSIMMetadata(ctx context.Context) (*SIMMetadata, error) {
	return b.readSIMMetadata(ctx)
}

func (b *MBIMBackend) GetOperatingMode(ctx context.Context) (OperatingMode, error) {
	rs, err := b.source.RadioState(ctx)
	if err != nil {
		return ModeLowPower, err
	}
	if rs.Software == mbim.RadioOn {
		return ModeOnline, nil
	}
	return ModeLowPower, nil
}

func (b *MBIMBackend) SetOperatingMode(ctx context.Context, mode OperatingMode) error {
	sw := mbim.RadioOff
	if mode == ModeOnline {
		sw = mbim.RadioOn
	}
	_, err := b.source.SetRadioState(ctx, sw)
	return err
}

func (b *MBIMBackend) AttachPacketService(ctx context.Context) error {
	_, err := b.source.SetPacketService(ctx, mbim.PacketServiceAttach)
	return err
}

func (b *MBIMBackend) DetachPacketService(ctx context.Context) error {
	_, err := b.source.SetPacketService(ctx, mbim.PacketServiceDetach)
	return err
}

func (b *MBIMBackend) RequestCoreRecovery(reason string) bool {
	type coreRecoveryRequester interface {
		RequestCoreRecovery(reason string) bool
	}
	if b == nil || b.source == nil {
		return false
	}
	requester, ok := b.source.(coreRecoveryRequester)
	return ok && requester.RequestCoreRecovery(reason)
}

func (b *MBIMBackend) WaitCoreReady(ctx context.Context) error {
	type coreReadyWaiter interface {
		WaitCoreReady(ctx context.Context) error
	}
	if b == nil || b.source == nil {
		return fmt.Errorf("mbim_source_not_available")
	}
	waiter, ok := b.source.(coreReadyWaiter)
	if !ok {
		return fmt.Errorf("mbim_core_ready_wait_not_supported")
	}
	return waiter.WaitCoreReady(ctx)
}

func (b *MBIMBackend) Reboot(ctx context.Context) error {
	caps := b.source.Capability()
	if caps == nil || !caps.DeviceResetUsable() {
		return fmt.Errorf("mbim device reset not supported")
	}
	return b.source.DeviceReset(ctx)
}

func (b *MBIMBackend) GetUIMReadiness(ctx context.Context) (manager.UIMReadiness, error) {
	return b.source.GetUIMReadiness(ctx)
}

func (b *MBIMBackend) UIMPowerOffSIM(ctx context.Context, slot uint8) error {
	return b.source.UIMPowerOffSIM(ctx, slot)
}

func (b *MBIMBackend) UIMPowerOnSIM(ctx context.Context, slot uint8) error {
	return b.source.UIMPowerOnSIM(ctx, slot)
}

func atou16(s string) uint16 {
	n, _ := strconv.Atoi(s)
	return uint16(n)
}

func mapMBIMRegisterState(s uint32) (int, string) {
	switch s {
	case 3:
		return 1, "registered-home"
	case 4, 5:
		return 5, "registered-roaming"
	case 2:
		return 2, "searching"
	case 6:
		return 3, "denied"
	case 1:
		return 0, "not-registered"
	default:
		return 4, "unknown"
	}
}
