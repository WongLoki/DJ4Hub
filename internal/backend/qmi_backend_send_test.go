package backend

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WongLoki/dj-4g-hub/internal/modem"
	"github.com/WongLoki/dj-4g-hub/pkg/smscodec"
	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/warthog618/sms/encoding/tpdu"
)

type qmiBackendSendSourceStub struct {
	lastFormat       uint8
	lastPDU          []byte
	sendCount        int
	powerOffSlots    []uint8
	powerOnSlots     []uint8
	uimReadiness     qmimanager.UIMReadiness
	uimReadinessErr  error
	servingCalls     int
	servingSeq       []*qmi.ServingSystem
	servingErrAt     map[int]error
	rfBandInfo       *qmi.RFBandInfo
	cellLocation     *qmi.CellLocationInfo
	getMSISDN        func(ctx context.Context) (string, error)
	nasPrefs         []qmi.SystemSelectionPreference
	deleteByTagCalls []string
	euiccOpenCalls   int
	scanResults      []qmi.NetworkScanResult
	scanErr          error
	incrementalInfo  *qmi.NASIncrementalNetworkScanInfo
	incrementalTS    time.Time
	incrementalOK    bool
	getSysSelPref    *qmi.SystemSelectionPreference
	initiateRegReqs  []qmi.NASInitiateNetworkRegisterRequest
}

func (s *qmiBackendSendSourceStub) GetDeviceSerialNumbers(ctx context.Context) (*qmi.DeviceInfo, error) {
	return &qmi.DeviceInfo{}, nil
}

func (s *qmiBackendSendSourceStub) GetDeviceRevision(ctx context.Context) (string, string, error) {
	return "", "", nil
}

func (s *qmiBackendSendSourceStub) GetIMSI(ctx context.Context) (string, error) {
	return "", nil
}

func (s *qmiBackendSendSourceStub) GetICCID(ctx context.Context) (string, error) {
	return "", nil
}

func (s *qmiBackendSendSourceStub) GetMSISDN(ctx context.Context) (string, error) {
	if s.getMSISDN != nil {
		return s.getMSISDN(ctx)
	}
	return "", nil
}

func (s *qmiBackendSendSourceStub) GetSIMStatus(ctx context.Context) (qmi.SIMStatus, error) {
	return qmi.SIMReady, nil
}

func (s *qmiBackendSendSourceStub) GetServingSystem(ctx context.Context) (*qmi.ServingSystem, error) {
	s.servingCalls++
	if s.servingErrAt != nil {
		if err, ok := s.servingErrAt[s.servingCalls]; ok {
			return nil, err
		}
	}
	if len(s.servingSeq) > 0 {
		resp := s.servingSeq[0]
		s.servingSeq = s.servingSeq[1:]
		if resp == nil {
			return &qmi.ServingSystem{}, nil
		}
		return resp, nil
	}
	return &qmi.ServingSystem{}, nil
}

func (s *qmiBackendSendSourceStub) GetSignalStrength(ctx context.Context) (*qmi.SignalStrength, error) {
	return &qmi.SignalStrength{}, nil
}

func (s *qmiBackendSendSourceStub) GetSignalInfo(ctx context.Context) (*qmi.SignalInfo, error) {
	return &qmi.SignalInfo{}, nil
}

func (s *qmiBackendSendSourceStub) NASGetRFBandInfo(ctx context.Context) (*qmi.RFBandInfo, error) {
	return s.rfBandInfo, nil
}

func (s *qmiBackendSendSourceStub) NASGetCellLocationInfo(ctx context.Context) (*qmi.CellLocationInfo, error) {
	return s.cellLocation, nil
}

func (s *qmiBackendSendSourceStub) NASInitiateNetworkRegister(ctx context.Context, req qmi.NASInitiateNetworkRegisterRequest) error {
	s.initiateRegReqs = append(s.initiateRegReqs, req)
	return nil
}

func (s *qmiBackendSendSourceStub) NASPerformNetworkScan(ctx context.Context) ([]qmi.NetworkScanResult, error) {
	return s.scanResults, s.scanErr
}

func (s *qmiBackendSendSourceStub) NASIncrementalNetworkScanSnapshot() (*qmi.NASIncrementalNetworkScanInfo, time.Time, bool) {
	return s.incrementalInfo, s.incrementalTS, s.incrementalOK
}

func (s *qmiBackendSendSourceStub) NASGetSystemSelectionPreference(ctx context.Context) (*qmi.SystemSelectionPreference, error) {
	if s.getSysSelPref != nil {
		return s.getSysSelPref, nil
	}
	return &qmi.SystemSelectionPreference{}, nil
}

func (s *qmiBackendSendSourceStub) NASForceNetworkSearch(ctx context.Context) error {
	return nil
}

func (s *qmiBackendSendSourceStub) NASAttachDetach(ctx context.Context, attached bool) error {
	return nil
}

func (s *qmiBackendSendSourceStub) NASSetSystemSelectionPreference(ctx context.Context, pref qmi.SystemSelectionPreference) error {
	s.nasPrefs = append(s.nasPrefs, pref)
	return nil
}

func (s *qmiBackendSendSourceStub) NASGetOperatorName(ctx context.Context) (*qmi.NASOperatorNameInfo, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) NASGetPLMNName(ctx context.Context, req qmi.NASPLMNNameRequest) (*qmi.NASPLMNNameInfo, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) NASConfigSignalInfoV2(ctx context.Context, cfg qmi.NASSignalInfoConfigV2) error {
	return nil
}

func (s *qmiBackendSendSourceStub) NASRegisterIndications(ctx context.Context, cfg qmi.NASIndicationRegistration) error {
	return nil
}

func (s *qmiBackendSendSourceStub) GetSysInfo(ctx context.Context) (*qmi.SysInfo, error) {
	return &qmi.SysInfo{}, nil
}

func (s *qmiBackendSendSourceStub) GetOperatingMode(ctx context.Context) (qmi.OperatingMode, error) {
	return qmi.ModeOnline, nil
}

func (s *qmiBackendSendSourceStub) SetOperatingMode(ctx context.Context, mode qmi.OperatingMode) error {
	return nil
}

func (s *qmiBackendSendSourceStub) WMSSendRawMessage(ctx context.Context, format uint8, pdu []byte) error {
	s.lastFormat = format
	s.lastPDU = append([]byte(nil), pdu...)
	s.sendCount++
	return nil
}

func (s *qmiBackendSendSourceStub) WMSRawReadMessage(ctx context.Context, storageType uint8, index uint32) ([]byte, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) WMSDeleteMessage(ctx context.Context, storageType uint8, index uint32) error {
	return nil
}

func (s *qmiBackendSendSourceStub) WMSListMessagesAuto(ctx context.Context, storageType uint8) ([]struct {
	Index uint32
	Tag   qmi.MessageTagType
}, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) WMSDeleteMessagesByTag(ctx context.Context, storageType uint8, tag qmi.MessageTagType, mode qmi.MessageMode) error {
	s.deleteByTagCalls = append(s.deleteByTagCalls, fmt.Sprintf("%d:%d:%d", storageType, tag, mode))
	return nil
}

func (s *qmiBackendSendSourceStub) OpenEUICCLogicalChannel(ctx context.Context, slot byte, aid []byte) (byte, error) {
	s.euiccOpenCalls++
	return 0, nil
}

func (s *qmiBackendSendSourceStub) CloseEUICCLogicalChannel(ctx context.Context, slot byte, channel byte) error {
	return nil
}

func (s *qmiBackendSendSourceStub) TransmitEUICCAPDU(ctx context.Context, slot byte, channel byte, command []byte) ([]byte, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) UIMPowerOffSIM(ctx context.Context, slot uint8) error {
	s.powerOffSlots = append(s.powerOffSlots, slot)
	return nil
}

func (s *qmiBackendSendSourceStub) UIMPowerOnSIM(ctx context.Context, slot uint8) error {
	s.powerOnSlots = append(s.powerOnSlots, slot)
	return nil
}

func (s *qmiBackendSendSourceStub) UIMPostSwitchReload(ctx context.Context, readiness qmimanager.UIMReadiness, opts qmimanager.UIMPostSwitchReloadOptions) (uint8, error) {
	s.powerOffSlots = append(s.powerOffSlots, opts.DefaultSlot)
	return opts.DefaultSlot, nil
}

func (s *qmiBackendSendSourceStub) GetUIMReadiness(ctx context.Context) (qmimanager.UIMReadiness, error) {
	return s.uimReadiness, s.uimReadinessErr
}

func TestQMIOperatingModeToBackendTreatsShutdownAsLowPower(t *testing.T) {
	if got := qmiOperatingModeToBackend(qmi.ModeShutdown); got != ModeLowPower {
		t.Fatalf("qmiOperatingModeToBackend(ModeShutdown)=%d want %d", got, ModeLowPower)
	}
}

func TestQMIOperatingModeToBackendTreatsOfflineAsLowPower(t *testing.T) {
	if got := qmiOperatingModeToBackend(qmi.ModeOffline); got != ModeLowPower {
		t.Fatalf("qmiOperatingModeToBackend(ModeOffline)=%d want %d", got, ModeLowPower)
	}
}

func TestModemUSSDResultMapsAPIFields(t *testing.T) {
	in := &modem.USSDResult{
		Status:  1,
		Text:    "balance",
		RawText: "00620061006c0061006e00630065",
		DCS:     72,
	}

	got := modemUSSDResult(in)
	if got == nil {
		t.Fatal("modemUSSDResult() returned nil")
	}
	if got.Status != in.Status || got.Text != in.Text || got.RawText != in.RawText || got.DCS != in.DCS {
		t.Fatalf("modemUSSDResult()=%+v want fields from %+v", got, in)
	}
	if got := modemUSSDResult(nil); got != nil {
		t.Fatalf("modemUSSDResult(nil)=%+v want nil", got)
	}
}

type qmiBackendUSSDSourceStub struct {
	qmiBackendSendSourceStub
	originateRequests []qmi.VoiceUSSDRequest
	cancelCalls       int
	events            []string
	originateDeadline bool
	deliver           bool
	deliverNoWait     bool
	blockOriginate    bool
	response          *qmi.VoiceUSSDResponse
	responseErr       error
	noWait            *qmi.VoiceUSSDNoWaitIndication
	systemSelection   *qmi.SystemSelectionPreference
	voiceConfig       *qmi.VoiceConfig
	initErr           error
	onUSSD            func(*qmi.VoiceUSSDIndication)
	onRelease         func()
	onNoWait          func(*qmi.VoiceUSSDNoWaitIndication)
}

type qmiBackendUSSDNoWaitSourceStub struct {
	*qmiBackendUSSDSourceStub
	noWaitRequests []qmi.VoiceUSSDRequest
	noWaitErr      error
}

func (s *qmiBackendUSSDNoWaitSourceStub) VOICEOriginateUSSDNoWait(ctx context.Context, req qmi.VoiceUSSDRequest) error {
	s.events = append(s.events, "originate-no-wait")
	s.noWaitRequests = append(s.noWaitRequests, req)
	if s.noWaitErr != nil {
		return s.noWaitErr
	}
	if s.deliverNoWait {
		go func() {
			if s.onNoWait != nil {
				s.onNoWait(s.noWait)
			}
		}()
	}
	return nil
}

func (s *qmiBackendUSSDSourceStub) VOICEOriginateUSSD(ctx context.Context, req qmi.VoiceUSSDRequest) (*qmi.VoiceUSSDResponse, error) {
	if _, ok := ctx.Deadline(); ok {
		s.originateDeadline = true
	}
	s.events = append(s.events, "originate")
	s.originateRequests = append(s.originateRequests, req)
	if s.blockOriginate {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.deliver {
		go func() {
			if s.onUSSD != nil {
				s.onUSSD(&qmi.VoiceUSSDIndication{
					USSData: &qmi.VoiceUSSDPayload{DCS: req.DCS, Text: "balance 10"},
				})
			}
		}()
	}
	if s.deliverNoWait {
		go func() {
			if s.onNoWait != nil {
				s.onNoWait(s.noWait)
			}
		}()
	}
	if s.response != nil {
		return s.response, s.responseErr
	}
	return &qmi.VoiceUSSDResponse{}, s.responseErr
}

func (s *qmiBackendUSSDSourceStub) NASGetSystemSelectionPreference(ctx context.Context) (*qmi.SystemSelectionPreference, error) {
	s.events = append(s.events, "nas-get")
	return s.systemSelection, nil
}

func (s *qmiBackendUSSDSourceStub) NASSetSystemSelectionPreference(ctx context.Context, pref qmi.SystemSelectionPreference) error {
	s.events = append(s.events, "nas-set")
	s.nasPrefs = append(s.nasPrefs, pref)
	return nil
}

func (s *qmiBackendUSSDSourceStub) VOICEGetConfig(ctx context.Context, query qmi.VoiceConfigQuery) (*qmi.VoiceConfig, error) {
	s.events = append(s.events, "voice-get-config")
	return s.voiceConfig, nil
}

func (s *qmiBackendUSSDSourceStub) VOICECancelUSSD(ctx context.Context) error {
	s.cancelCalls++
	return nil
}

func (s *qmiBackendUSSDSourceStub) OnVoiceUSSD(h func(*qmi.VoiceUSSDIndication)) error {
	if s.initErr != nil {
		return s.initErr
	}
	s.onUSSD = h
	return nil
}

func (s *qmiBackendUSSDSourceStub) OnVoiceUSSDReleased(h func()) error {
	if s.initErr != nil {
		return s.initErr
	}
	s.onRelease = h
	return nil
}

func (s *qmiBackendUSSDSourceStub) OnVoiceUSSDNoWaitResult(h func(*qmi.VoiceUSSDNoWaitIndication)) error {
	if s.initErr != nil {
		return s.initErr
	}
	s.onNoWait = h
	return nil
}

func TestQMIBackendExecuteUSSDReturnsVoiceResult(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{deliver: true}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if got.Text != "balance 10" || got.DCS != 0x01 {
		t.Fatalf("ExecuteUSSD()=%+v want text and dcs", got)
	}
	if len(src.originateRequests) != 1 {
		t.Fatalf("originate calls=%d want 1", len(src.originateRequests))
	}
	req := src.originateRequests[0]
	if req.DCS != 0x01 || string(req.Data) != "*100#" {
		t.Fatalf("USSD request=%+v want dcs=0x01 data=*100#", req)
	}
}

func TestQMIBackendExecuteUSSDTimeoutCancelsSession(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{deliver: false}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", 5*time.Millisecond)
	if err == nil {
		t.Fatal("ExecuteUSSD() error=nil want timeout")
	}
	if src.cancelCalls != 1 {
		t.Fatalf("cancelCalls=%d want 1", src.cancelCalls)
	}
}

func TestQMIBackendExecuteUSSDReturnsCallbackRegistrationError(t *testing.T) {
	initErr := fmt.Errorf("voice callbacks unavailable")
	src := &qmiBackendUSSDSourceStub{initErr: initErr}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err == nil || !strings.Contains(err.Error(), initErr.Error()) {
		t.Fatalf("ExecuteUSSD() error=%v want callback init error", err)
	}
	if len(src.originateRequests) != 0 {
		t.Fatalf("originate calls=%d want 0 after callback init failure", len(src.originateRequests))
	}
}

func TestQMIBackendExecuteUSSDPrefersNoWaitOriginate(t *testing.T) {
	src := &qmiBackendUSSDNoWaitSourceStub{
		qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{
			deliverNoWait: true,
			noWait: &qmi.VoiceUSSDNoWaitIndication{
				USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "balance 10"},
			},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if got.Text != "balance 10" || got.DCS != 0x01 {
		t.Fatalf("ExecuteUSSD()=%+v want no-wait result", got)
	}
	if len(src.noWaitRequests) != 1 {
		t.Fatalf("VOICEOriginateUSSDNoWait calls=%d want 1", len(src.noWaitRequests))
	}
	if len(src.originateRequests) != 0 {
		t.Fatalf("VOICEOriginateUSSD calls=%d want 0 when no-wait succeeds", len(src.originateRequests))
	}
}

func TestQMIBackendExecuteUSSDFallsBackWhenNoWaitUnsupported(t *testing.T) {
	src := &qmiBackendUSSDNoWaitSourceStub{
		qmiBackendUSSDSourceStub: &qmiBackendUSSDSourceStub{
			response: &qmi.VoiceUSSDResponse{
				USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "sync result"},
			},
		},
		noWaitErr: fmt.Errorf("originate ussd no wait failed: %w", &qmi.QMIError{
			Service:   qmi.ServiceVOICE,
			MessageID: qmi.VOICEOriginateUSSDNoWait,
			Result:    0x0001,
			ErrorCode: qmi.QMIErrInvalidQmiCmd,
		}),
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if got.Text != "sync result" {
		t.Fatalf("ExecuteUSSD()=%+v want sync fallback result", got)
	}
	if len(src.noWaitRequests) != 1 || len(src.originateRequests) != 1 {
		t.Fatalf("noWait calls=%d sync calls=%d want 1 each", len(src.noWaitRequests), len(src.originateRequests))
	}
}

func TestQMIBackendExecuteUSSDReturnsSynchronousVoiceResult(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		response: &qmi.VoiceUSSDResponse{
			USSData: &qmi.VoiceUSSDPayload{DCS: 0x0F, Data: []byte("raw sync"), Text: "sync result"},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if got.Text != "sync result" || got.RawText != "raw sync" || got.DCS != 0x0F {
		t.Fatalf("ExecuteUSSD()=%+v want synchronous response payload", got)
	}
	if src.cancelCalls != 0 {
		t.Fatalf("cancelCalls=%d want 0 for synchronous result", src.cancelCalls)
	}
}

func TestQMIBackendExecuteUSSDIncludesResponseFailureCauseOnQMIError(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		response: &qmi.VoiceUSSDResponse{
			FailureCause:    0x007d,
			HasFailureCause: true,
		},
		responseErr: fmt.Errorf("originate ussd failed: %w", &qmi.QMIError{
			Service:   qmi.ServiceVOICE,
			MessageID: qmi.VOICEOriginateUSSD,
			Result:    0x0001,
			ErrorCode: 0x005c,
		}),
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err == nil {
		t.Fatal("ExecuteUSSD() error=nil want QMI failure")
	}
	text := err.Error()
	for _, want := range []string{"SUPS_FAILURE_CASE", "failure_cause=0x007D", "SUPSSystemFailure"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ExecuteUSSD() error=%q missing %q", text, want)
		}
	}
}

func TestQMIBackendExecuteUSSDNamesWrongStateFailureCause(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		response: &qmi.VoiceUSSDResponse{
			FailureCause:    0x00da,
			HasFailureCause: true,
		},
		responseErr: fmt.Errorf("originate ussd failed: %w", &qmi.QMIError{
			Service:   qmi.ServiceVOICE,
			MessageID: qmi.VOICEOriginateUSSD,
			Result:    0x0001,
			ErrorCode: 0x005c,
		}),
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err == nil {
		t.Fatal("ExecuteUSSD() error=nil want QMI failure")
	}
	text := err.Error()
	for _, want := range []string{"SUPS_FAILURE_CASE", "failure_cause=0x00DA", "VOICEWrongState", "MMGMMWrongState"} {
		if !strings.Contains(text, want) {
			t.Fatalf("ExecuteUSSD() error=%q missing %q", text, want)
		}
	}
}

func TestQMIBackendExecuteUSSDPreparesCSVoiceDomainWhenPSOnly(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		systemSelection: &qmi.SystemSelectionPreference{
			ServiceDomainPreference:    qmi.NASServiceDomainPreferencePSOnly,
			HasServiceDomainPreference: true,
			VoiceDomainPreference:      qmi.NASVoiceDomainPreferencePSOnly,
			HasVoiceDomainPreference:   true,
		},
		voiceConfig: &qmi.VoiceConfig{
			CurrentVoiceDomainPreference:    uint8(qmi.NASVoiceDomainPreferencePSOnly),
			HasCurrentVoiceDomainPreference: true,
		},
		response: &qmi.VoiceUSSDResponse{
			USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "balance 10"},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if got.Text != "balance 10" {
		t.Fatalf("ExecuteUSSD()=%+v want synchronous response", got)
	}
	if len(src.nasPrefs) != 1 {
		t.Fatalf("NASSetSystemSelectionPreference calls=%d want 1", len(src.nasPrefs))
	}
	pref := src.nasPrefs[0]
	if !pref.HasServiceDomainPreference || pref.ServiceDomainPreference != qmi.NASServiceDomainPreferenceCSPS {
		t.Fatalf("service domain pref=%+v want CS+PS", pref)
	}
	if !pref.HasVoiceDomainPreference || pref.VoiceDomainPreference != qmi.NASVoiceDomainPreferenceCSPreferred {
		t.Fatalf("voice domain pref=%+v want CS preferred", pref)
	}
	if !pref.HasChangeDuration || pref.ChangeDuration != qmi.NASChangeDurationPowerCycle {
		t.Fatalf("change duration pref=%+v want power-cycle scoped", pref)
	}
	if gotEvents, wantEvents := strings.Join(src.events, ","), "nas-get,voice-get-config,nas-set,originate"; gotEvents != wantEvents {
		t.Fatalf("events=%s want %s", gotEvents, wantEvents)
	}
}

func TestQMIBackendExecuteUSSDPreparesCSVoiceDomainWhenPSPreferred(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		systemSelection: &qmi.SystemSelectionPreference{
			ServiceDomainPreference:    qmi.NASServiceDomainPreferenceCSPS,
			HasServiceDomainPreference: true,
			VoiceDomainPreference:      qmi.NASVoiceDomainPreferencePSPreferred,
			HasVoiceDomainPreference:   true,
		},
		voiceConfig: &qmi.VoiceConfig{
			CurrentVoiceDomainPreference:    uint8(qmi.NASVoiceDomainPreferenceCSOnly),
			HasCurrentVoiceDomainPreference: true,
		},
		response: &qmi.VoiceUSSDResponse{
			USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "balance 10"},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	if _, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second); err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if len(src.nasPrefs) != 1 {
		t.Fatalf("NASSetSystemSelectionPreference calls=%d want 1", len(src.nasPrefs))
	}
	pref := src.nasPrefs[0]
	if pref.HasServiceDomainPreference {
		t.Fatalf("service domain pref=%+v want unchanged", pref)
	}
	if !pref.HasVoiceDomainPreference || pref.VoiceDomainPreference != qmi.NASVoiceDomainPreferenceCSPreferred {
		t.Fatalf("voice domain pref=%+v want CS preferred", pref)
	}
	if gotEvents, wantEvents := strings.Join(src.events, ","), "nas-get,voice-get-config,nas-set,originate"; gotEvents != wantEvents {
		t.Fatalf("events=%s want %s", gotEvents, wantEvents)
	}
}

func TestQMIBackendExecuteUSSDUsesVoiceConfigWhenSystemSelectionOmitsVoiceDomain(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		systemSelection: &qmi.SystemSelectionPreference{
			ServiceDomainPreference:    qmi.NASServiceDomainPreferenceCSPS,
			HasServiceDomainPreference: true,
		},
		voiceConfig: &qmi.VoiceConfig{
			CurrentVoiceDomainPreference:    uint8(qmi.NASVoiceDomainPreferencePSOnly),
			HasCurrentVoiceDomainPreference: true,
		},
		response: &qmi.VoiceUSSDResponse{
			USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "balance 10"},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	if _, err := be.ExecuteUSSD(context.Background(), "*100#", time.Second); err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if len(src.nasPrefs) != 1 {
		t.Fatalf("NASSetSystemSelectionPreference calls=%d want 1", len(src.nasPrefs))
	}
	pref := src.nasPrefs[0]
	if pref.HasServiceDomainPreference {
		t.Fatalf("service domain pref=%+v want unchanged", pref)
	}
	if !pref.HasVoiceDomainPreference || pref.VoiceDomainPreference != qmi.NASVoiceDomainPreferenceCSPreferred {
		t.Fatalf("voice domain pref=%+v want CS preferred", pref)
	}
	if gotEvents, wantEvents := strings.Join(src.events, ","), "nas-get,voice-get-config,nas-set,originate"; gotEvents != wantEvents {
		t.Fatalf("events=%s want %s", gotEvents, wantEvents)
	}
}

func TestQMIBackendExecuteUSSDAppliesRequestTimeoutToOriginate(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		response: &qmi.VoiceUSSDResponse{
			USSData: &qmi.VoiceUSSDPayload{DCS: 0x01, Text: "balance 10"},
		},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	if _, err := be.ExecuteUSSD(context.Background(), "*100#", 2*time.Second); err != nil {
		t.Fatalf("ExecuteUSSD() error=%v", err)
	}
	if !src.originateDeadline {
		t.Fatal("VOICEOriginateUSSD context had no deadline")
	}
}

func TestQMIBackendExecuteUSSDOriginateDeadlineReturnsNetworkTimeout(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{blockOriginate: true}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}
	parentCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = be.ExecuteUSSD(parentCtx, "*100#", 5*time.Millisecond)
	if err == nil {
		t.Fatal("ExecuteUSSD() error=nil want timeout")
	}
	if !strings.Contains(err.Error(), "USSD 响应网络超时") {
		t.Fatalf("ExecuteUSSD() error=%v want USSD network timeout", err)
	}
	if strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("ExecuteUSSD() error=%v should not leak raw context deadline", err)
	}
	if src.cancelCalls != 1 {
		t.Fatalf("cancelCalls=%d want 1", src.cancelCalls)
	}
}

func TestQMIBackendExecuteUSSDReturnsNoWaitFailure(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{
		deliverNoWait: true,
		noWait:        &qmi.VoiceUSSDNoWaitIndication{HasErrorCode: true, ErrorCode: 0x1234},
	}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", time.Second)
	if err == nil || !strings.Contains(err.Error(), "0x1234") {
		t.Fatalf("ExecuteUSSD() error=%v want no-wait error code", err)
	}
	if src.cancelCalls != 0 {
		t.Fatalf("cancelCalls=%d want 0 for explicit no-wait failure", src.cancelCalls)
	}
}

func TestQMIBackendExecuteUSSDRejectsNextSessionUntilTimedOutSessionReleases(t *testing.T) {
	src := &qmiBackendUSSDSourceStub{deliver: false}
	be, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	_, err = be.ExecuteUSSD(context.Background(), "*100#", 5*time.Millisecond)
	if err == nil {
		t.Fatal("first ExecuteUSSD() error=nil want timeout")
	}
	if src.onUSSD == nil || src.onRelease == nil {
		t.Fatal("USSD callbacks were not registered")
	}

	src.onUSSD(&qmi.VoiceUSSDIndication{USSData: &qmi.VoiceUSSDPayload{DCS: 0x0F, Text: "stale previous result"}})
	_, err = be.ExecuteUSSD(context.Background(), "*101#", time.Second)
	if err == nil || !strings.Contains(err.Error(), "上次 USSD 会话尚未释放") {
		t.Fatalf("second ExecuteUSSD() error=%v want unreleased-session error", err)
	}
	if len(src.originateRequests) != 1 {
		t.Fatalf("originate calls=%d want only first request before release", len(src.originateRequests))
	}

	src.deliver = true
	src.onRelease()
	got, err := be.ExecuteUSSD(context.Background(), "*101#", time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD() after release error=%v", err)
	}
	if got.Text != "balance 10" {
		t.Fatalf("ExecuteUSSD() after release=%+v want fresh result", got)
	}
	if len(src.originateRequests) != 2 {
		t.Fatalf("originate calls=%d want second request after release", len(src.originateRequests))
	}
}

func TestQMIUSSDPayloadResultKeepsRawPayloadData(t *testing.T) {
	got := qmiUSSDPayloadResult(1, &qmi.VoiceUSSDPayload{
		DCS:  0x0F,
		Data: []byte("raw balance"),
		Text: "balance 10",
	})

	if got.Text != "balance 10" || got.RawText != "raw balance" || got.Status != 1 || got.DCS != 0x0F {
		t.Fatalf("qmiUSSDPayloadResult()=%+v want decoded text and raw payload data", got)
	}
}

func (s *qmiBackendSendSourceStub) GetNativeMCCMNC(ctx context.Context) (string, string, error) {
	return "", "", nil
}

func (s *qmiBackendSendSourceStub) GetNativeSPN(ctx context.Context) (string, error) {
	return "", nil
}

func (s *qmiBackendSendSourceStub) GetSIMMetadata(ctx context.Context) (*qmi.SIMMetadata, error) {
	return nil, nil
}

func (s *qmiBackendSendSourceStub) GetSMSC(ctx context.Context) (string, error) {
	return "", nil
}

// GetDeviceSnapshot 返回 nil，表示没有快照，会降级到实时 IPC。
func (s *qmiBackendSendSourceStub) GetDeviceSnapshot() *qmimanager.DeviceSnapshot {
	return nil
}

type qmiBackendSIMAuthChannelSourceStub struct {
	qmiBackendSendSourceStub
	simAuthOpenAID []byte
	simAuthCloseCh byte
	simAuthUSIMAID []byte
	simAuthISIMAID []byte
	simAuthUSIMErr error
	simAuthISIMErr error
}

func (s *qmiBackendSIMAuthChannelSourceStub) OpenSIMAuthLogicalChannel(ctx context.Context, slot byte, aid []byte) (byte, error) {
	s.simAuthOpenAID = append([]byte(nil), aid...)
	return 7, nil
}

func (s *qmiBackendSIMAuthChannelSourceStub) CloseSIMAuthLogicalChannel(ctx context.Context, slot byte, channel byte) error {
	s.simAuthCloseCh = channel
	return nil
}

func (s *qmiBackendSIMAuthChannelSourceStub) GetUSIMAID(ctx context.Context) ([]byte, error) {
	if s.simAuthUSIMErr != nil {
		return nil, s.simAuthUSIMErr
	}
	return append([]byte(nil), s.simAuthUSIMAID...), nil
}

func (s *qmiBackendSIMAuthChannelSourceStub) GetISIMAID(ctx context.Context) ([]byte, error) {
	if s.simAuthISIMErr != nil {
		return nil, s.simAuthISIMErr
	}
	return append([]byte(nil), s.simAuthISIMAID...), nil
}

func TestQMIBackendSIMAuthLogicalChannelUsesSIMAuthSource(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	ch, err := backend.OpenLogicalChannel(context.Background(), "A0000000871002")
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if ch != 7 {
		t.Fatalf("OpenLogicalChannel() = %d, want 7", ch)
	}
	if got := strings.ToUpper(hex.EncodeToString(src.simAuthOpenAID)); got != "A0000000871002" {
		t.Fatalf("SIMAuth open AID = %s", got)
	}

	if err := backend.CloseLogicalChannel(context.Background(), 7); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}
	if src.simAuthCloseCh != 7 {
		t.Fatalf("SIMAuth close channel = %d, want 7", src.simAuthCloseCh)
	}
}

func TestQMIBackendResolveSIMAuthAIDUsesFullUSIMAID(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{
		simAuthUSIMAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02, 0xFF, 0x49, 0xFF, 0x01, 0x89},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	aid, source, err := backend.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID() error = %v", err)
	}
	if aid != "A0000000871002FF49FF0189" {
		t.Fatalf("resolved aid = %s, want full USIM AID", aid)
	}
	if source != "qmi_card_status" {
		t.Fatalf("source = %s, want qmi_card_status", source)
	}
}

func TestQMIBackendResolveSIMAuthAIDUsesFullISIMAID(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{
		simAuthISIMAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0xFF, 0xFF, 0xFF, 0xFF, 0x89, 0x03, 0x02, 0x00, 0x00},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	aid, source, err := backend.ResolveSIMAuthAID(context.Background(), "isim", "A0000000871004")
	if err != nil {
		t.Fatalf("ResolveSIMAuthAID() error = %v", err)
	}
	if aid != "A0000000871004FFFFFFFF8903020000" {
		t.Fatalf("resolved aid = %s, want full ISIM AID", aid)
	}
	if source != "qmi_card_status" {
		t.Fatalf("source = %s, want qmi_card_status", source)
	}
}

func TestQMIBackendResolveSIMAuthAIDReturnsNotReadyOnQMITimeout(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{simAuthUSIMErr: errors.New("context deadline exceeded")}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	aid, source, err := backend.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002")
	if err == nil {
		t.Fatal("ResolveSIMAuthAID() err=nil, want not-ready")
	}
	if aid != "" || !strings.Contains(source, "not_ready") {
		t.Fatalf("aid=%q source=%q, want empty not-ready result", aid, source)
	}
}

func TestQMIBackendResolveSIMAuthAIDRejectsShortAID(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{
		simAuthUSIMAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	aid, source, err := backend.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002")
	if err == nil {
		t.Fatal("ResolveSIMAuthAID() err=nil, want not-ready")
	}
	if aid != "" || !strings.Contains(source, "not_ready") {
		t.Fatalf("aid=%q source=%q, want empty not-ready result", aid, source)
	}
}

func TestQMIBackendResolveSIMAuthAIDRejectsMismatchedAID(t *testing.T) {
	src := &qmiBackendSIMAuthChannelSourceStub{
		simAuthUSIMAID: []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04, 0xFF, 0x49, 0xFF, 0x01, 0x89},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	aid, source, err := backend.ResolveSIMAuthAID(context.Background(), "usim", "A0000000871002")
	if err == nil {
		t.Fatal("ResolveSIMAuthAID() err=nil, want not-ready")
	}
	if aid != "" || !strings.Contains(source, "not_ready") {
		t.Fatalf("aid=%q source=%q, want empty not-ready result", aid, source)
	}
}

func TestQMIBackendSIMAuthLogicalChannelRequiresSIMAuthSource(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if _, err := backend.OpenLogicalChannel(context.Background(), "A0000000871002"); err == nil {
		t.Fatal("OpenLogicalChannel() error=nil, want missing SIMAuth logical channel source")
	}
	if src.euiccOpenCalls != 0 {
		t.Fatalf("EUICC logical channel fallback calls=%d want 0", src.euiccOpenCalls)
	}
}

func TestQMIBackendSendSMSUsesDefaultSMSCHeader(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if err := backend.SendSMS(context.Background(), "10086", "x"); err != nil {
		t.Fatalf("SendSMS failed: %v", err)
	}
	if src.sendCount != 1 {
		t.Fatalf("expected 1 send call, got %d", src.sendCount)
	}
	if src.lastFormat != 0x06 {
		t.Fatalf("unexpected format 0x%02x", src.lastFormat)
	}
	if len(src.lastPDU) < 5 {
		t.Fatalf("PDU too short: %d", len(src.lastPDU))
	}
	if src.lastPDU[0] != 0x00 {
		t.Fatalf("expected default SMSC marker 0x00, got 0x%02x", src.lastPDU[0])
	}
	if src.lastPDU[4] != 0x81 {
		t.Fatalf("short-code TOA should be 0x81, got 0x%02x", src.lastPDU[4])
	}
}

func TestQMIBackendSendSMSWithOptionsForcesUCS2(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if err := backend.SendSMSWithOptions(context.Background(), "10086", "hello", smscodec.SubmitOptions{Encoding: smscodec.SMSEncodingUCS2}); err != nil {
		t.Fatalf("SendSMSWithOptions failed: %v", err)
	}
	if src.sendCount != 1 {
		t.Fatalf("expected 1 send call, got %d", src.sendCount)
	}
	if len(src.lastPDU) < 2 || src.lastPDU[0] != 0x00 {
		t.Fatalf("unexpected PDU with SMSC header: %x", src.lastPDU)
	}

	pdu := &tpdu.TPDU{Direction: tpdu.MO}
	if err := pdu.UnmarshalBinary(src.lastPDU[1:]); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if pdu.DCS != tpdu.DcsUCS2Data {
		t.Fatalf("DCS=0x%02x want 0x%02x", byte(pdu.DCS), byte(tpdu.DcsUCS2Data))
	}
}

func TestQMIBackendDeleteAllSMSClearsReadAndUnreadFromBothStorages(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if err := backend.DeleteAllSMS(context.Background()); err != nil {
		t.Fatalf("DeleteAllSMS() error=%v", err)
	}

	want := []string{
		"0:0:1",
		"0:1:1",
		"1:0:1",
		"1:1:1",
	}
	if !reflect.DeepEqual(src.deleteByTagCalls, want) {
		t.Fatalf("deleteByTagCalls=%#v want %#v", src.deleteByTagCalls, want)
	}
}

func TestQMIBackendUIMPowerControlPassThrough(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if err := backend.UIMPowerOffSIM(context.Background(), 1); err != nil {
		t.Fatalf("UIMPowerOffSIM failed: %v", err)
	}
	if err := backend.UIMPowerOnSIM(context.Background(), 1); err != nil {
		t.Fatalf("UIMPowerOnSIM failed: %v", err)
	}

	if len(src.powerOffSlots) != 1 || src.powerOffSlots[0] != 1 {
		t.Fatalf("powerOffSlots=%v want [1]", src.powerOffSlots)
	}
	if len(src.powerOnSlots) != 1 || src.powerOnSlots[0] != 1 {
		t.Fatalf("powerOnSlots=%v want [1]", src.powerOnSlots)
	}
}

func TestQMIBackend_UIMPostSwitchReload(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	slot, err := backend.UIMPostSwitchReload(context.Background(), qmimanager.UIMReadiness{}, qmimanager.UIMPostSwitchReloadOptions{DefaultSlot: 1})
	if err != nil {
		t.Fatalf("UIMPostSwitchReload() error=%v", err)
	}
	if slot != 1 {
		t.Fatalf("slot=%d want 1", slot)
	}
}

func TestQMIBackendGetUIMReadinessPassesThroughSource(t *testing.T) {
	src := &qmiBackendSendSourceStub{
		uimReadiness: qmimanager.UIMReadiness{
			TransportReady: true,
			ControlReady:   true,
			UIMReady:       true,
			SIMStatus:      qmi.SIMReady,
			ActiveSlot:     2,
			SlotKnown:      true,
			Reason:         qmimanager.UIMReadinessReady,
			ICCID:          "8985203103011907194",
		},
	}
	backend, err := NewQMIBackend("/dev/cdc-wdm1", src)
	if err != nil {
		t.Fatalf("NewQMIBackend() error=%v", err)
	}

	got, err := backend.GetUIMReadiness(context.Background())
	if err != nil {
		t.Fatalf("GetUIMReadiness() error=%v", err)
	}
	if got.ActiveSlot != 2 || got.Reason != qmimanager.UIMReadinessReady || got.ICCID != "8985203103011907194" {
		t.Fatalf("readiness=%+v", got)
	}
}

func TestQMIBackendNASSetSystemSelectionAutomaticUsesPermanentDuration(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	if err := backend.NASSetSystemSelectionAutomatic(context.Background()); err != nil {
		t.Fatalf("NASSetSystemSelectionAutomatic failed: %v", err)
	}
	if len(src.nasPrefs) != 1 {
		t.Fatalf("nasPrefs=%d want 1", len(src.nasPrefs))
	}
	pref := src.nasPrefs[0]
	if !pref.HasNetworkSelectionPreference || pref.NetworkSelectionPreference != qmi.NASNetworkSelectionAutomatic {
		t.Fatalf("network selection has=%v value=%d want automatic", pref.HasNetworkSelectionPreference, pref.NetworkSelectionPreference)
	}
	if !pref.HasChangeDuration || pref.ChangeDuration != qmi.NASChangeDurationPermanent {
		t.Fatalf("change duration has=%v value=%d want permanent", pref.HasChangeDuration, pref.ChangeDuration)
	}
}

func TestQMIBackendGetServingSystemFallbackWhenRegisteredButOperatorEmpty(t *testing.T) {
	src := &qmiBackendSendSourceStub{
		servingSeq: []*qmi.ServingSystem{
			{
				RegistrationState: qmi.RegStateRegistered,
				PSAttached:        true,
				RadioInterface:    0x08,
				MCC:               0,
				MNC:               0,
			},
			{
				RegistrationState: qmi.RegStateRegistered,
				PSAttached:        true,
				RadioInterface:    0x08,
				MCC:               460,
				MNC:               1,
			},
		},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	ss, err := backend.GetServingSystem(context.Background())
	if err != nil {
		t.Fatalf("GetServingSystem failed: %v", err)
	}
	if src.servingCalls != 2 {
		t.Fatalf("servingCalls=%d want=2", src.servingCalls)
	}
	if ss.RegStatus != 1 || !ss.PSAttached || ss.NetworkMode != "LTE" {
		t.Fatalf("unexpected serving status: %+v", ss)
	}
	if ss.Operator == "" {
		t.Fatalf("expected non-empty operator after fallback, got %+v", ss)
	}
}

func TestQMIBackendGetServingSystemAddsLTEDuplexWithoutChangingMode(t *testing.T) {
	src := &qmiBackendSendSourceStub{
		servingSeq: []*qmi.ServingSystem{
			{
				RegistrationState: qmi.RegStateRegistered,
				PSAttached:        true,
				RadioInterface:    0x08,
				MCC:               460,
				MNC:               1,
			},
		},
		rfBandInfo: &qmi.RFBandInfo{Bands: []qmi.RFBandInfoEntry{{RadioInterface: 0x08, ActiveBandClass: 8}}},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	ss, err := backend.GetServingSystem(context.Background())
	if err != nil {
		t.Fatalf("GetServingSystem failed: %v", err)
	}
	if ss.NetworkMode != "LTE" {
		t.Fatalf("NetworkMode=%q want LTE", ss.NetworkMode)
	}
	if ss.NetworkDuplex != "FDD" {
		t.Fatalf("NetworkDuplex=%q want FDD", ss.NetworkDuplex)
	}
}

func TestQMIBackendGetServingSystemFallbackFailureKeepsCurrentStatus(t *testing.T) {
	src := &qmiBackendSendSourceStub{
		servingSeq: []*qmi.ServingSystem{
			{
				RegistrationState: qmi.RegStateRegistered,
				PSAttached:        true,
				RadioInterface:    0x08,
				MCC:               0,
				MNC:               0,
			},
		},
		servingErrAt: map[int]error{
			2: context.DeadlineExceeded,
		},
	}
	backend, err := NewQMIBackend("/dev/null", src)
	if err != nil {
		t.Fatalf("NewQMIBackend failed: %v", err)
	}

	ss, err := backend.GetServingSystem(context.Background())
	if err != nil {
		t.Fatalf("GetServingSystem failed: %v", err)
	}
	if src.servingCalls != 2 {
		t.Fatalf("servingCalls=%d want=2", src.servingCalls)
	}
	if ss.RegStatus != 1 || ss.Operator != "" {
		t.Fatalf("unexpected fallback-degraded status: %+v", ss)
	}
}

func TestQMIBackend_SetOperatorSelection_Manual(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	be, _ := NewQMIBackend("/dev/null", src)

	pcs := true
	req := SetOperatorSelectionRequest{
		Mode:             OperatorSelectionManual,
		MCC:              "310",
		MNC:              "260",
		IncludesPCSDigit: &pcs,
		PLMN:             "310260",
	}

	_, err := be.SetOperatorSelection(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(src.nasPrefs) != 0 {
		t.Fatalf("manual selection should not set system selection preference, got %d calls", len(src.nasPrefs))
	}

	if len(src.initiateRegReqs) != 1 {
		t.Fatalf("expected 1 reg req, got %d", len(src.initiateRegReqs))
	}
	reg := src.initiateRegReqs[0]
	if reg.Mode != qmi.NASNetworkRegisterManual || reg.MCC != 310 || reg.MNC != 260 || !reg.IncludesPCSDigit {
		t.Fatalf("reg req mismatch: %+v", reg)
	}
}

func TestQMIBackend_SetOperatorSelection_ManualDerivesMCCMNCFromPLMN(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	be, _ := NewQMIBackend("/dev/null", src)

	req := SetOperatorSelectionRequest{
		Mode: OperatorSelectionManual,
		PLMN: "46000",
		RAT:  OperatorRATLTE,
	}

	_, err := be.SetOperatorSelection(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(src.nasPrefs) != 0 {
		t.Fatalf("manual selection should not set system selection preference, got %d calls", len(src.nasPrefs))
	}

	if len(src.initiateRegReqs) != 1 {
		t.Fatalf("expected 1 reg req, got %d", len(src.initiateRegReqs))
	}
	reg := src.initiateRegReqs[0]
	if reg.Mode != qmi.NASNetworkRegisterManual || reg.MCC != 460 || reg.MNC != 0 || reg.IncludesPCSDigit || reg.RadioAccessTech != 0x08 {
		t.Fatalf("reg req mismatch: %+v", reg)
	}
}

func TestQMIBackend_SetOperatorSelection_Automatic(t *testing.T) {
	src := &qmiBackendSendSourceStub{}
	be, _ := NewQMIBackend("/dev/null", src)

	req := SetOperatorSelectionRequest{
		Mode: OperatorSelectionAutomatic,
	}

	_, err := be.SetOperatorSelection(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(src.nasPrefs) != 1 {
		t.Fatalf("expected 1 pref set, got %d", len(src.nasPrefs))
	}
	if src.nasPrefs[0].NetworkSelectionPreference != qmi.NASNetworkSelectionAutomatic {
		t.Fatalf("pref mismatch: %+v", src.nasPrefs[0])
	}

	if len(src.initiateRegReqs) != 1 {
		t.Fatalf("expected 1 reg req, got %d", len(src.initiateRegReqs))
	}
	if src.initiateRegReqs[0].Mode != qmi.NASNetworkRegisterAutomatic {
		t.Fatalf("reg req mismatch: %+v", src.initiateRegReqs[0])
	}
}

func TestQMIBackend_ScanOperators(t *testing.T) {
	src := &qmiBackendSendSourceStub{
		scanResults: []qmi.NetworkScanResult{
			{MCC: "460", MNC: "00", Status: 1, Description: "China Mobile"},
			{MCC: "460", MNC: "01", Status: 2, Description: "China Unicom"},
			{MCC: "999", MNC: "99", Status: 3, Description: "Forbidden Net"},
		},
	}
	be, _ := NewQMIBackend("/dev/null", src)

	candidates, err := be.ScanOperators(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}

	if candidates[0].Status != "current" {
		t.Fatalf("expected current status, got %s", candidates[0].Status)
	}
	if candidates[1].Status != "available" {
		t.Fatalf("expected available status, got %s", candidates[1].Status)
	}
	if candidates[2].Status != "forbidden" {
		t.Fatalf("expected forbidden status, got %s", candidates[2].Status)
	}
}

func TestQMIBackend_IncrementalOperatorScanSnapshot(t *testing.T) {
	ts := time.Now()
	src := &qmiBackendSendSourceStub{
		incrementalInfo: &qmi.NASIncrementalNetworkScanInfo{
			ScanComplete: true,
			Results: []qmi.NetworkScanResult{
				{MCC: "460", MNC: "00", Status: 1, Description: "China Mobile"},
				{MCC: "460", MNC: "01", Status: 2, Description: "China Unicom"},
			},
		},
		incrementalTS: ts,
		incrementalOK: true,
	}
	be, _ := NewQMIBackend("/dev/null", src)

	candidates, complete, gotTS, ok := be.IncrementalOperatorScanSnapshot()
	if !ok {
		t.Fatal("ok=false want true")
	}
	if !complete {
		t.Fatal("complete=false want true")
	}
	if !gotTS.Equal(ts) {
		t.Fatalf("timestamp=%v want %v", gotTS, ts)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates)=%d want 2", len(candidates))
	}
	if candidates[0].Status != "current" || candidates[1].Status != "available" {
		t.Fatalf("unexpected candidates=%+v", candidates)
	}
}

func (s *qmiBackendSendSourceStub) EnsureSIMProvisioned(ctx context.Context, opts qmimanager.EnsureSIMProvisionedOptions) (qmimanager.UIMReadiness, error) {
	return qmimanager.UIMReadiness{}, nil
}

func (s *qmiBackendUSSDSourceStub) EnsureSIMProvisioned(ctx context.Context, opts qmimanager.EnsureSIMProvisionedOptions) (qmimanager.UIMReadiness, error) {
	return qmimanager.UIMReadiness{}, nil
}

func (s *qmiBackendUSSDNoWaitSourceStub) EnsureSIMProvisioned(ctx context.Context, opts qmimanager.EnsureSIMProvisionedOptions) (qmimanager.UIMReadiness, error) {
	return qmimanager.UIMReadiness{}, nil
}

func (s *qmiBackendSIMAuthChannelSourceStub) EnsureSIMProvisioned(ctx context.Context, opts qmimanager.EnsureSIMProvisionedOptions) (qmimanager.UIMReadiness, error) {
	return qmimanager.UIMReadiness{}, nil
}
