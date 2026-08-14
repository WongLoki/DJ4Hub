package backend

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/pkg/mbim"
	"github.com/WongLoki/DJ4Hub/pkg/smscodec"
	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/warthog618/sms/encoding/tpdu"
)

type fakeMBIMSource struct {
	caps              mbim.Caps
	capability        *mbim.Capabilities
	sub               mbim.SubscriberReady
	reg               mbim.RegisterState
	setReg            mbim.RegisterState
	sig               mbim.SignalState
	pkt               mbim.PacketService
	packetAction      mbim.PacketServiceAction
	packetSet         mbim.PacketService
	packetErr         error
	radio             mbim.RadioState
	setSw             mbim.RadioSwitch
	providers         []mbim.Provider
	homeProvider      mbim.Provider
	homeProviderErr   error
	setRegisterAction uint32
	setRegisterPLMN   string
	uimReadiness      qmimanager.UIMReadiness
	uimReadinessErr   error
	powerOffCalls     int
	powerOnCalls      int

	sendFn     func(pdu []byte) (uint32, error)
	listFn     func() ([]mbim.SMSRecord, error)
	openFn     func([]byte) (uint32, error)
	apduFn     func(uint32, []byte) ([]byte, error)
	smscFn     func(context.Context) (string, error)
	setSMSCArg string
	efFn       func(uint16) ([]byte, error)
	efRecFn    func(uint16, uint32) ([]byte, error)
	aidFn      func([]byte) ([]byte, error)

	resetCalled bool
	resetErr    error

	ussdResult          mbim.USSDResult
	ussdErr             error
	ussdCommand         string
	ussdTimeout         time.Duration
	ussdContinueInput   string
	ussdContinueTimeout time.Duration
	cancelUSSD          bool
	cancelErr           error

	akaRES []byte
	akaCK  []byte
	akaIK  []byte
	akaErr error
}

func (f *fakeMBIMSource) ReadSIMRecordEF(_ context.Context, fileID uint16, record uint32) ([]byte, error) {
	if f.efRecFn != nil {
		return f.efRecFn(fileID, record)
	}
	return nil, nil
}

func (f *fakeMBIMSource) CalculateAKA(_ context.Context, rand, autn []byte) (res, ik, ck, auts []byte, err error) {
	return f.akaRES, f.akaIK, f.akaCK, nil, f.akaErr
}

func (f *fakeMBIMSource) ControlDevice() string                         { return "/dev/cdc-wdm0" }
func (f *fakeMBIMSource) DeviceCaps(context.Context) (mbim.Caps, error) { return f.caps, nil }
func (f *fakeMBIMSource) SubscriberReady(context.Context) (mbim.SubscriberReady, error) {
	return f.sub, nil
}
func (f *fakeMBIMSource) RegisterState(context.Context) (mbim.RegisterState, error) {
	return f.reg, nil
}
func (f *fakeMBIMSource) GetRegisterState(context.Context) (mbim.RegisterState, error) {
	return f.reg, nil
}
func (f *fakeMBIMSource) VisibleProviders(context.Context) ([]mbim.Provider, error) {
	return f.providers, nil
}
func (f *fakeMBIMSource) HomeProvider(context.Context) (mbim.Provider, error) {
	return f.homeProvider, f.homeProviderErr
}
func (f *fakeMBIMSource) GetUIMReadiness(context.Context) (qmimanager.UIMReadiness, error) {
	return f.uimReadiness, f.uimReadinessErr
}
func (f *fakeMBIMSource) UIMPowerOffSIM(context.Context, uint8) error {
	f.powerOffCalls++
	return nil
}
func (f *fakeMBIMSource) UIMPowerOnSIM(context.Context, uint8) error {
	f.powerOnCalls++
	return nil
}

// 原运营商应取 HomeProvider 的 PLMN(MNC 长度正确,3 位 → 6 位),而不是 IMSI 截 2 位。
func TestMBIMBackendGetNativeMCCMNCUsesHomeProviderLength(t *testing.T) {
	src := &fakeMBIMSource{
		sub:          mbim.SubscriberReady{IMSI: "310840131414639"}, // IMSI[3:5]="84"(错)
		homeProvider: mbim.Provider{PLMN: "310840"},                 // 正确 3 位 MNC
	}
	b := NewMBIMBackend("", src)

	mcc, mnc, err := b.GetNativeMCCMNC(context.Background())
	if err != nil {
		t.Fatalf("GetNativeMCCMNC: %v", err)
	}
	if mcc != "310" || mnc != "840" {
		t.Fatalf("GetNativeMCCMNC = (%q,%q), want (310,840)", mcc, mnc)
	}
}

// 真机场景:HomeProvider 拿不到、USIM ADF 也开不了(EF_AD 读失败),
// 仍应靠 IMSI+MCC 表得到正确的 3 位 MNC(310→280),而不是截成 2 位。
func TestMBIMBackendGetNativeMCCMNCUsesMCCTableWhenNoHomeProviderNoEFAD(t *testing.T) {
	src := &fakeMBIMSource{
		sub:             mbim.SubscriberReady{IMSI: "310280233688494"},
		homeProviderErr: fmt.Errorf("home provider unavailable"),
		efFn:            func(uint16) ([]byte, error) { return nil, fmt.Errorf("UICC_OPEN_CHANNEL status=0x87430002") },
	}
	b := NewMBIMBackend("", src)

	mcc, mnc, err := b.GetNativeMCCMNC(context.Background())
	if err != nil {
		t.Fatalf("GetNativeMCCMNC: %v", err)
	}
	if mcc != "310" || mnc != "280" {
		t.Fatalf("GetNativeMCCMNC = (%q,%q), want (310,280)", mcc, mnc)
	}
}

// HomeProvider 不可用时,退回 IMSI(取 2 位 MNC),保证不 panic、有兜底。
func TestMBIMBackendGetNativeMCCMNCFallsBackToIMSI(t *testing.T) {
	src := &fakeMBIMSource{
		sub:             mbim.SubscriberReady{IMSI: "460001234567890"},
		homeProviderErr: fmt.Errorf("home provider unavailable"),
	}
	b := NewMBIMBackend("", src)

	mcc, mnc, err := b.GetNativeMCCMNC(context.Background())
	if err != nil {
		t.Fatalf("GetNativeMCCMNC: %v", err)
	}
	if mcc != "460" || mnc != "00" {
		t.Fatalf("GetNativeMCCMNC fallback = (%q,%q), want (460,00)", mcc, mnc)
	}
}
func (f *fakeMBIMSource) SetRegister(_ context.Context, action uint32, plmn string) (mbim.RegisterState, error) {
	f.setRegisterAction = action
	f.setRegisterPLMN = plmn
	if f.setReg.ProviderID != "" || f.setReg.RegisterState != 0 {
		return f.setReg, nil
	}
	return f.reg, nil
}
func (f *fakeMBIMSource) SignalState(context.Context) (mbim.SignalState, error) {
	return f.sig, nil
}
func (f *fakeMBIMSource) PacketService(context.Context) (mbim.PacketService, error) {
	return f.pkt, nil
}
func (f *fakeMBIMSource) SetPacketService(_ context.Context, action mbim.PacketServiceAction) (mbim.PacketService, error) {
	f.packetAction = action
	return f.packetSet, f.packetErr
}
func (f *fakeMBIMSource) RadioState(context.Context) (mbim.RadioState, error) { return f.radio, nil }
func (f *fakeMBIMSource) SetRadioState(_ context.Context, sw mbim.RadioSwitch) (mbim.RadioState, error) {
	f.setSw = sw
	return mbim.RadioState{Software: sw}, nil
}
func (f *fakeMBIMSource) Snapshot() mbim.Snapshot        { return mbim.Snapshot{} }
func (f *fakeMBIMSource) Capability() *mbim.Capabilities { return f.capability }
func (f *fakeMBIMSource) SendSMS(_ context.Context, pdu []byte) (uint32, error) {
	if f.sendFn != nil {
		return f.sendFn(pdu)
	}
	return 0, nil
}
func (f *fakeMBIMSource) ReadSMS(context.Context, uint32) (mbim.SMSRecord, error) {
	return mbim.SMSRecord{}, nil
}
func (f *fakeMBIMSource) ListSMS(context.Context) ([]mbim.SMSRecord, error) {
	if f.listFn != nil {
		return f.listFn()
	}
	return nil, nil
}
func (f *fakeMBIMSource) DeleteSMS(context.Context, uint32) error { return nil }
func (f *fakeMBIMSource) DeleteAllSMS(context.Context) error      { return nil }
func (f *fakeMBIMSource) GetSMSC(ctx context.Context) (string, error) {
	if f.smscFn != nil {
		return f.smscFn(ctx)
	}
	return "", nil
}
func (f *fakeMBIMSource) SetSMSC(_ context.Context, smsc string) error {
	f.setSMSCArg = smsc
	return nil
}
func (f *fakeMBIMSource) ExecuteUSSD(_ context.Context, command string, timeout time.Duration) (mbim.USSDResult, error) {
	f.ussdCommand = command
	f.ussdTimeout = timeout
	return f.ussdResult, f.ussdErr
}
func (f *fakeMBIMSource) ContinueUSSD(_ context.Context, input string, timeout time.Duration) (mbim.USSDResult, error) {
	f.ussdContinueInput = input
	f.ussdContinueTimeout = timeout
	return f.ussdResult, f.ussdErr
}
func (f *fakeMBIMSource) CancelUSSD(context.Context) error {
	f.cancelUSSD = true
	return f.cancelErr
}
func (f *fakeMBIMSource) OpenChannel(_ context.Context, aid []byte) (uint32, error) {
	if f.openFn != nil {
		return f.openFn(aid)
	}
	return 0, nil
}
func (f *fakeMBIMSource) CloseChannel(context.Context, uint32) error { return nil }
func (f *fakeMBIMSource) TransmitAPDU(_ context.Context, ch uint32, cmd []byte) ([]byte, error) {
	if f.apduFn != nil {
		return f.apduFn(ch, cmd)
	}
	return nil, nil
}
func (f *fakeMBIMSource) ReadSIMEF(_ context.Context, fileID uint16, _ int) ([]byte, error) {
	if f.efFn != nil {
		return f.efFn(fileID)
	}
	return nil, nil
}
func (f *fakeMBIMSource) ResolveAppAID(_ context.Context, prefix []byte) ([]byte, error) {
	if f.aidFn != nil {
		return f.aidFn(prefix)
	}
	return nil, fmt.Errorf("ResolveAppAID 未配置")
}

func (f *fakeMBIMSource) ResolveLogicalChannelAID(app string, fallback string) (string, string, error) {
	if f.aidFn != nil {
		prefix := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
		if app == "isim" {
			prefix = []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
		}
		res, err := f.aidFn(prefix)
		if err != nil {
			return fallback, "fallback_on_error", err
		}
		return fmt.Sprintf("%X", res), "mock", nil
	}
	return fallback, "mock", nil
}
func (f *fakeMBIMSource) DeviceReset(context.Context) error {
	f.resetCalled = true
	return f.resetErr
}

// liveIdentityReader mirrors device.liveSIMIdentityReader/liveSIMSPNReader/
// liveSIMMetadataReader so we assert MBIMBackend satisfies the contract the
// worker's refreshIdentityLive relies on (regression guard for empty ICCID/IMSI).
type liveIdentityReader interface {
	GetICCIDLive(ctx context.Context) (string, error)
	GetIMSILive(ctx context.Context) (string, error)
	GetNativeSPNLive(ctx context.Context) (string, error)
	GetSIMMetadataLive(ctx context.Context) (*SIMMetadata, error)
}

func TestMBIMBackendLiveIdentity(t *testing.T) {
	src := &fakeMBIMSource{
		sub: mbim.SubscriberReady{ReadyState: 1, IMSI: "310840131414639", ICCID: "89103000000589140892"},
	}
	var b any = NewMBIMBackend("", src)
	reader, ok := b.(liveIdentityReader)
	if !ok {
		t.Fatal("MBIMBackend 未实现 live 身份接口（会导致面板 ICCID/IMSI 为空）")
	}
	if iccid, _ := reader.GetICCIDLive(context.Background()); iccid != "89103000000589140892" {
		t.Fatalf("GetICCIDLive = %q", iccid)
	}
	if imsi, _ := reader.GetIMSILive(context.Background()); imsi != "310840131414639" {
		t.Fatalf("GetIMSILive = %q", imsi)
	}
}

func TestMBIMBackendIdentity(t *testing.T) {
	src := &fakeMBIMSource{
		caps: mbim.Caps{DeviceID: "356938035643809"},
		sub:  mbim.SubscriberReady{ReadyState: 1, IMSI: "460001234567890", ICCID: "8986001234", MSISDN: "13800138000"},
	}
	b := NewMBIMBackend("", src)
	if b.Mode() != BackendMBIM {
		t.Fatalf("Mode = %q", b.Mode())
	}
	if imei, _ := b.GetIMEI(context.Background()); imei != "356938035643809" {
		t.Fatalf("IMEI = %q", imei)
	}
	if imsi, _ := b.GetIMSI(context.Background()); imsi != "460001234567890" {
		t.Fatalf("IMSI = %q", imsi)
	}
	ok, _ := b.IsSimInserted(context.Background())
	if !ok {
		t.Fatal("ReadyState=1 should be inserted")
	}
}

func TestMBIMDataClassToNetworkMode(t *testing.T) {
	cases := []struct {
		dataClass uint32
		want      string
	}{
		{0x00, ""},
		{0x01, "GSM"},        // GPRS
		{0x02, "GSM"},        // EDGE
		{0x04, "UMTS"},       // UMTS
		{0x08, "UMTS"},       // HSDPA
		{0x10, "UMTS"},       // HSUPA
		{0x20, "LTE"},        // LTE
		{0x40, "NR5G"},       // 5G NSA
		{0x80, "NR5G"},       // 5G SA
		{0x20 | 0x08, "LTE"}, // 取最高 RAT
		{0x40 | 0x20, "NR5G"},
	}
	for _, c := range cases {
		if got := mbimDataClassToNetworkMode(c.dataClass); got != c.want {
			t.Errorf("mbimDataClassToNetworkMode(0x%x) = %q, want %q", c.dataClass, got, c.want)
		}
	}
}

func TestMBIMBackendServingSystemAndSignal(t *testing.T) {
	src := &fakeMBIMSource{
		reg: mbim.RegisterState{RegisterState: 3, ProviderName: "CMCC", MCC: "460", MNC: "00"},
		sig: mbim.SignalState{RSSI: 20, DBM: -73},
		pkt: mbim.PacketService{State: 2, HighestClass: 0x20},
	}
	b := NewMBIMBackend("", src)
	ss, err := b.GetServingSystem(context.Background())
	if err != nil {
		t.Fatalf("GetServingSystem: %v", err)
	}
	if ss.RegStatus != 1 || ss.Operator != "CMCC" || ss.MCC != 460 || ss.MNC != 0 || !ss.PSAttached {
		t.Fatalf("serving = %+v", ss)
	}
	if ss.NetworkMode != "LTE" {
		t.Fatalf("NetworkMode = %q, want LTE", ss.NetworkMode)
	}
	si, err := b.GetSignalInfo(context.Background())
	if err != nil {
		t.Fatalf("GetSignalInfo: %v", err)
	}
	if si.RSSI != -73 {
		t.Fatalf("RSSI = %d, want -73", si.RSSI)
	}
	if ss.LAC != "" || ss.CellID != "" || ss.RadioBand != "" || ss.RadioChannel != 0 {
		t.Fatalf("MBIM serving system should not fabricate LAC/CellID/Band/Channel: %+v", ss)
	}
	if si.RSRQ != 0 {
		t.Fatalf("MBIM signal info should not fabricate RSRQ: %+v", si)
	}
}

func TestMBIMBackendAttachPacketServiceDelegates(t *testing.T) {
	src := &fakeMBIMSource{packetSet: mbim.PacketService{State: 2}}
	b := NewMBIMBackend("", src)

	if err := b.AttachPacketService(context.Background()); err != nil {
		t.Fatalf("AttachPacketService: %v", err)
	}
	if src.packetAction != mbim.PacketServiceAttach {
		t.Fatalf("packet action=%d want attach", src.packetAction)
	}
}

func TestMBIMBackendServingSystemFallsBackToPLMNOperatorName(t *testing.T) {
	src := &fakeMBIMSource{
		reg: mbim.RegisterState{RegisterState: 3, ProviderName: "", MCC: "460", MNC: "00"},
		pkt: mbim.PacketService{State: 2, HighestClass: 0x20},
	}
	b := NewMBIMBackend("", src)

	ss, err := b.GetServingSystem(context.Background())
	if err != nil {
		t.Fatalf("GetServingSystem: %v", err)
	}
	if ss.Operator == "" {
		t.Fatalf("Operator empty, want PLMN fallback name or PLMN display: %+v", ss)
	}
	if ss.RadioBand != "" || ss.RadioChannel != 0 || ss.NetworkDuplex != "" {
		t.Fatalf("MBIM must not fabricate band/channel/duplex: %+v", ss)
	}
}

func TestMBIMBackendSignalInfoFillsRsrpSinrFromV2(t *testing.T) {
	src := &fakeMBIMSource{
		sig: mbim.SignalState{RSSI: 99, Unknown: true, RSRP: -85, HasRSRP: true, SNR: 10, HasSNR: true},
	}
	b := NewMBIMBackend("", src)
	si, err := b.GetSignalInfo(context.Background())
	if err != nil {
		t.Fatalf("GetSignalInfo: %v", err)
	}
	if si.RSRP != -85 {
		t.Fatalf("RSRP = %d, want -85", si.RSRP)
	}
	if si.SINR != 10 {
		t.Fatalf("SINR = %d, want 10", si.SINR)
	}
}

func TestMBIMBackendOperatingMode(t *testing.T) {
	src := &fakeMBIMSource{radio: mbim.RadioState{Software: mbim.RadioOn}}
	b := NewMBIMBackend("", src)
	mode, _ := b.GetOperatingMode(context.Background())
	if mode != ModeOnline {
		t.Fatalf("mode = %d, want online", mode)
	}
	if err := b.SetOperatingMode(context.Background(), ModeLowPower); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if src.setSw != mbim.RadioOff {
		t.Fatal("ModeLowPower should turn radio off")
	}
}

func TestMBIMBackendSendSMS(t *testing.T) {
	var sent [][]byte
	src := &fakeMBIMSource{}
	src.sendFn = func(pdu []byte) (uint32, error) { sent = append(sent, pdu); return 1, nil }
	b := NewMBIMBackend("", src)
	if err := b.SendSMS(context.Background(), "+8613800138000", "hello"); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if len(sent) == 0 {
		t.Fatal("no PDU sent")
	}
	if sent[0][0] != 0x00 {
		t.Fatalf("PDU first byte should be SMSC length 0x00, got 0x%02x", sent[0][0])
	}
}

func TestMBIMBackendSendSMSWithOptionsForcesUCS2(t *testing.T) {
	var sent [][]byte
	src := &fakeMBIMSource{}
	src.sendFn = func(pdu []byte) (uint32, error) { sent = append(sent, pdu); return 1, nil }
	b := NewMBIMBackend("", src)
	if err := b.SendSMSWithOptions(context.Background(), "10086", "hello", smscodec.SubmitOptions{Encoding: smscodec.SMSEncodingUCS2}); err != nil {
		t.Fatalf("SendSMSWithOptions: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if len(sent[0]) < 2 || sent[0][0] != 0x00 {
		t.Fatalf("unexpected PDU with SMSC header: %x", sent[0])
	}
	pdu := &tpdu.TPDU{Direction: tpdu.MO}
	if err := pdu.UnmarshalBinary(sent[0][1:]); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if pdu.DCS != tpdu.DcsUCS2Data {
		t.Fatalf("DCS=0x%02x want 0x%02x", byte(pdu.DCS), byte(tpdu.DcsUCS2Data))
	}
}

func TestMBIMBackendCapabilityDelegates(t *testing.T) {
	want := &mbim.Capabilities{UICCChannelOK: true}
	src := &fakeMBIMSource{capability: want}
	b := NewMBIMBackend("", src)
	if b.Capability() != want {
		t.Fatal("Capability 应透传 source 的能力对象")
	}
}

func TestMBIMBackendGetUIMReadinessDelegates(t *testing.T) {
	src := &fakeMBIMSource{
		uimReadiness: qmimanager.UIMReadiness{
			TransportReady: true,
			ControlReady:   true,
			UIMReady:       true,
			ActiveSlot:     1,
			SlotKnown:      true,
			ICCID:          "8986001234",
			Reason:         qmimanager.UIMReadinessReady,
		},
	}
	b := NewMBIMBackend("", src)
	got, err := b.GetUIMReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetUIMReadiness: %v", err)
	}
	if got.ActiveSlot != 1 || got.ICCID != "8986001234" {
		t.Fatalf("got=%+v", got)
	}
}

func TestMBIMBackendUIMPowerDelegates(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)
	if err := b.UIMPowerOffSIM(context.Background(), 1); err != nil {
		t.Fatalf("UIMPowerOffSIM: %v", err)
	}
	if err := b.UIMPowerOnSIM(context.Background(), 1); err != nil {
		t.Fatalf("UIMPowerOnSIM: %v", err)
	}
	if src.powerOffCalls != 1 || src.powerOnCalls != 1 {
		t.Fatalf("off=%d on=%d", src.powerOffCalls, src.powerOnCalls)
	}
}

func TestMBIMBackendCalculateAKAMarksDeadOnNoDeviceSupport(t *testing.T) {
	caps := &mbim.Capabilities{Services: mbim.DeviceServices{
		Elements: []mbim.DeviceServiceElement{{Service: mbim.UUIDAuth, CIDs: []uint32{1}}},
	}}
	src := &fakeMBIMSource{
		capability: caps,
		akaErr:     &mbim.StatusError{Op: "AUTH_AKA", Status: 9}, // NO_DEVICE_SUPPORT
	}
	b := NewMBIMBackend("", src)
	if !caps.AuthAKAUsable() {
		t.Fatal("前置:应可用")
	}
	_, _, _, _, err := b.CalculateAKA(context.Background(), make([]byte, 16), make([]byte, 16))
	if err == nil {
		t.Fatal("应返回错误")
	}
	if caps.AuthAKAUsable() {
		t.Fatal("status=9(NO_DEVICE_SUPPORT) 后应熔断")
	}
}

func TestMBIMBackendCalculateAKADoesNotMarkDeadOnSyncFailure(t *testing.T) {
	caps := &mbim.Capabilities{Services: mbim.DeviceServices{
		Elements: []mbim.DeviceServiceElement{{Service: mbim.UUIDAuth, CIDs: []uint32{1}}},
	}}
	src := &fakeMBIMSource{
		capability: caps,
		akaErr:     &mbim.StatusError{Op: "AUTH_AKA", Status: 35}, // AUTH_SYNC_FAILURE
	}
	b := NewMBIMBackend("", src)
	_, _, _, _, err := b.CalculateAKA(context.Background(), make([]byte, 16), make([]byte, 16))
	if err == nil {
		t.Fatal("应返回错误")
	}
	if !caps.AuthAKAUsable() {
		t.Fatal("status=35(AUTH_SYNC_FAILURE) 是合法认证响应，不应熔断 Auth 服务")
	}
}

func TestMBIMBackendListSMS(t *testing.T) {
	src := &fakeMBIMSource{listFn: func() ([]mbim.SMSRecord, error) {
		return []mbim.SMSRecord{{Index: 3, Status: 1, PDU: []byte{0xAA}}}, nil
	}}
	b := NewMBIMBackend("", src)
	list, err := b.ListSMS(context.Background())
	if err != nil {
		t.Fatalf("ListSMS: %v", err)
	}
	if len(list) != 1 || list[0].Index != 3 || list[0].Tag != 1 {
		t.Fatalf("list = %+v", list)
	}
}

func TestMBIMBackendGetSMSCDelegates(t *testing.T) {
	src := &fakeMBIMSource{
		smscFn: func(context.Context) (string, error) {
			return "+8613800138000", nil
		},
	}
	var _ SMSCProvider = (*MBIMBackend)(nil)

	b := NewMBIMBackend("", src)
	smsc, err := b.GetSMSC(context.Background())
	if err != nil {
		t.Fatalf("GetSMSC: %v", err)
	}
	if smsc != "+8613800138000" {
		t.Fatalf("SMSC = %q, want %q", smsc, "+8613800138000")
	}
}

func TestMBIMBackendSetSMSCDelegates(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)
	if err := b.SetSMSC(context.Background(), "+8613800138000"); err != nil {
		t.Fatalf("SetSMSC: %v", err)
	}
	if src.setSMSCArg != "+8613800138000" {
		t.Fatalf("setSMSCArg = %q, want %q", src.setSMSCArg, "+8613800138000")
	}
}

func TestMBIMBackendSIMAuthDelegates(t *testing.T) {
	src := &fakeMBIMSource{
		openFn: func(aid []byte) (uint32, error) { return 1, nil },
		apduFn: func(ch uint32, cmd []byte) ([]byte, error) { return []byte{0x90, 0x00}, nil },
	}
	b := NewMBIMBackend("", src)
	ch, err := b.OpenLogicalChannel(context.Background(), "A0000000871002")
	if err != nil {
		t.Fatalf("OpenLogicalChannel: %v", err)
	}
	if ch != 1 {
		t.Fatalf("channel = %d", ch)
	}
	resp, err := b.TransmitAPDU(context.Background(), ch, "00A40004023F00")
	if err != nil {
		t.Fatalf("TransmitAPDU: %v", err)
	}
	if resp != "9000" {
		t.Fatalf("resp = %q, want 9000", resp)
	}
}

// ResolveSIMAuthAID 让 backend.SIMAuthAIDResolver 在 MBIM 下也可用。
// SIM 身份鉴权与 ATAKAProvider 风格的逻辑通道读取都依赖
// 这个接口拿完整 AID,而不是直接用拒绝短 AID 的卡上 fallback 短 AID 去开通道。
func TestMBIMBackendResolveSIMAuthAIDUSIM(t *testing.T) {
	full := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0x89, 0x07, 0x09, 0x00, 0x00}
	src := &fakeMBIMSource{aidFn: func(prefix []byte) ([]byte, error) {
		want := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}
		if !bytesEqual(prefix, want) {
			t.Fatalf("prefix = % X, want % X", prefix, want)
		}
		return full, nil
	}}
	b := NewMBIMBackend("", src)
	aid, source, err := b.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID: %v", err)
	}
	if aid != "A0000000871002FFFFFFFF8907090000" {
		t.Fatalf("aid = %q", aid)
	}
	if source == "" {
		t.Fatalf("source should not be empty")
	}
}

func TestMBIMBackendResolveSIMAuthAIDISIM(t *testing.T) {
	full := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0xFF, 0xFF, 0xFF, 0xFF, 0x89, 0x07, 0x09, 0x00, 0x00}
	src := &fakeMBIMSource{aidFn: func(prefix []byte) ([]byte, error) {
		want := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
		if !bytesEqual(prefix, want) {
			t.Fatalf("prefix = % X, want % X", prefix, want)
		}
		return full, nil
	}}
	b := NewMBIMBackend("", src)
	aid, _, err := b.ResolveSIMAuthAID(context.Background(), "isim", "A0000000871004")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID: %v", err)
	}
	if aid != "A0000000871004FFFFFFFF8907090000" {
		t.Fatalf("aid = %q", aid)
	}
}

func TestMBIMBackendResolveSIMAuthAIDFailsWhenUnresolved(t *testing.T) {
	src := &fakeMBIMSource{aidFn: func([]byte) ([]byte, error) {
		return nil, fmt.Errorf("mbim: UICC_APPLICATION_LIST status=0x9")
	}}
	b := NewMBIMBackend("", src)
	if _, _, err := b.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002"); err == nil {
		t.Fatalf("expected error when AID cannot be resolved")
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNormalizeAndValidateMBIM(t *testing.T) {
	if NormalizeBackendMode("MBIM") != BackendMBIM {
		t.Fatalf("normalize mbim failed: %q", NormalizeBackendMode("MBIM"))
	}
	if err := ValidateBackendMode("mbim"); err != nil {
		t.Fatalf("validate mbim should pass: %v", err)
	}
}

func TestNewBackendMBIM(t *testing.T) {
	src := &fakeMBIMSource{caps: mbim.Caps{DeviceID: "123"}}
	be, err := NewBackend(BackendMBIM, "/dev/cdc-wdm0", nil, nil, src)
	if err != nil {
		t.Fatalf("NewBackend(mbim): %v", err)
	}
	if be.Mode() != BackendMBIM {
		t.Fatalf("mode = %q", be.Mode())
	}
}

func TestMBIMBackendCalculateAKA(t *testing.T) {
	b := &MBIMBackend{source: &fakeMBIMSource{
		akaRES: []byte("res"),
		akaCK:  []byte("ck"),
		akaIK:  []byte("ik"),
	}}
	res, ik, ck, auts, err := b.CalculateAKA(context.Background(), make([]byte, 16), make([]byte, 16))
	if err != nil {
		t.Fatalf("CalculateAKA: %v", err)
	}
	if string(res) != "res" || string(ck) != "ck" || string(ik) != "ik" || len(auts) > 0 {
		t.Fatalf("Unexpected AKA result: res=%v, ck=%v, ik=%v, auts=%v", res, ck, ik, auts)
	}
}
