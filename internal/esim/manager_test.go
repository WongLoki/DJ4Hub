package esim

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/backend"
	"github.com/damonto/euicc-go/bertlv"
	"github.com/damonto/euicc-go/bertlv/primitive"
	"github.com/damonto/euicc-go/driver"
	euicchttp "github.com/damonto/euicc-go/http"
	"github.com/damonto/euicc-go/lpa"
	sgp22 "github.com/damonto/euicc-go/v2"
	qmiq "github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"golang.org/x/sync/singleflight"
)

func newTestManagerWithOverviewLoader(loader func() (*EsimOverview, error)) *Manager {
	return &Manager{
		deviceID:       "dev-esim",
		sf:             &singleflight.Group{},
		opDone:         make(chan struct{}),
		switchSignal:   make(chan string, 16),
		overviewLoader: loader,
	}
}

func newManagerWithChannelFactory(
	deviceID string,
	channelFactory func(aid []byte) (*lpa.Client, error),
	clearFn func(),
	onBefore func(),
	onAfter func(),
) *Manager {
	var beforeWithToken func(SwitchOperation, string) uint64
	if onBefore != nil {
		beforeWithToken = func(SwitchOperation, string) uint64 { onBefore(); return 0 }
	}
	var afterWithToken func(SwitchOperation, uint64)
	if onAfter != nil {
		afterWithToken = func(SwitchOperation, uint64) { onAfter() }
	}
	mgr := &Manager{
		deviceID:             deviceID,
		transport:            transportCustom,
		channelFactory:       channelFactory,
		clearChannels:        clearFn,
		sf:                   &singleflight.Group{},
		onBeforeSwitch:       beforeWithToken,
		onAfterSwitch:        afterWithToken,
		switchSignal:         make(chan string, 16),
		opDone:               make(chan struct{}),
		postSwitchMinDelay:   defaultPostSwitchMinDelay,
		readQueueWaitTimeout: defaultReadQueueWaitTimeout,
	}
	mgr.overviewLoader = mgr.loadOverviewFresh
	mgr.profilesLoader = mgr.loadProfilesFresh
	return mgr
}

type fakeAPDUIdleWaiter struct {
	wait func(ctx context.Context) error
}

func (f fakeAPDUIdleWaiter) WaitIdle(ctx context.Context) error {
	if f.wait != nil {
		return f.wait(ctx)
	}
	return nil
}

type fakeProfileOperationTransmitter struct {
	err                error
	calls              *atomic.Int32
	profiles           []*sgp22.ProfileInfo
	listCalls          *atomic.Int32
	eid                []byte
	onProfileOperation func()
}

func (f fakeProfileOperationTransmitter) Transmit(request bertlv.Marshaler, response bertlv.Unmarshaler) error {
	switch request.(type) {
	case *sgp22.ProfileOperationRequest:
		if f.calls != nil {
			f.calls.Add(1)
		}
		if f.onProfileOperation != nil {
			f.onProfileOperation()
		}
		return f.err
	case *sgp22.ProfileInfoListRequest:
		resp, ok := response.(*sgp22.ProfileInfoListResponse)
		if !ok {
			return errors.New("unexpected profile list response type")
		}
		if f.listCalls != nil {
			f.listCalls.Add(1)
		}
		resp.ProfileList = f.profiles
		return nil
	case *sgp22.GetEuiccDataRequest:
		resp, ok := response.(*sgp22.GetEuiccDataResponse)
		if !ok {
			return errors.New("unexpected eid response type")
		}
		resp.EID = append([]byte(nil), f.eid...)
		return nil
	default:
		return errors.New("unexpected request type")
	}
}

func (f fakeProfileOperationTransmitter) TransmitRaw(_ []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

type fakeSIMPowerBackend struct {
	powerOffSlots []uint8
	powerOnSlots  []uint8
	powerOffErr   error
	powerOnErr    error
	mode          string
	setModeCalls  []backend.OperatingMode
	setModeErr    error
	iccid         string
	iccidErr      error
	iccidCalls    atomic.Int32
}

func (f *fakeSIMPowerBackend) GetIMEI(ctx context.Context) (string, error) { return "", nil }
func (f *fakeSIMPowerBackend) GetIMSI(ctx context.Context) (string, error) { return "", nil }
func (f *fakeSIMPowerBackend) GetICCID(ctx context.Context) (string, error) {
	f.iccidCalls.Add(1)
	return f.iccid, f.iccidErr
}
func (f *fakeSIMPowerBackend) GetICCIDLive(ctx context.Context) (string, error) {
	return f.GetICCID(ctx)
}
func (f *fakeSIMPowerBackend) GetMSISDN(ctx context.Context) (string, error) {
	return "", nil
}
func (f *fakeSIMPowerBackend) GetRevision(ctx context.Context) (string, error) {
	return "", nil
}
func (f *fakeSIMPowerBackend) GetSignalInfo(ctx context.Context) (*backend.SignalInfo, error) {
	return nil, nil
}
func (f *fakeSIMPowerBackend) GetServingSystem(ctx context.Context) (*backend.ServingSystem, error) {
	return nil, nil
}
func (f *fakeSIMPowerBackend) IsSimInserted(ctx context.Context) (bool, error) {
	return true, nil
}
func (f *fakeSIMPowerBackend) GetNativeMCCMNC(ctx context.Context) (string, string, error) {
	return "", "", nil
}
func (f *fakeSIMPowerBackend) GetNativeSPN(ctx context.Context) (string, error) {
	return "", nil
}
func (f *fakeSIMPowerBackend) GetSIMMetadata(ctx context.Context) (*backend.SIMMetadata, error) {
	return nil, nil
}
func (f *fakeSIMPowerBackend) SendSMS(ctx context.Context, to, body string) error {
	return nil
}
func (f *fakeSIMPowerBackend) ReadSMS(ctx context.Context, index int) (*backend.SMS, error) {
	return nil, nil
}
func (f *fakeSIMPowerBackend) DeleteSMS(ctx context.Context, index int) error { return nil }
func (f *fakeSIMPowerBackend) ListSMS(ctx context.Context) ([]backend.SMSSummary, error) {
	return nil, nil
}
func (f *fakeSIMPowerBackend) DeleteAllSMS(ctx context.Context) error { return nil }
func (f *fakeSIMPowerBackend) SetOperatingMode(ctx context.Context, mode backend.OperatingMode) error {
	f.setModeCalls = append(f.setModeCalls, mode)
	return f.setModeErr
}
func (f *fakeSIMPowerBackend) GetOperatingMode(ctx context.Context) (backend.OperatingMode, error) {
	return backend.ModeOnline, nil
}
func (f *fakeSIMPowerBackend) Reboot(ctx context.Context) error { return nil }
func (f *fakeSIMPowerBackend) OpenLogicalChannel(ctx context.Context, aid string) (int, error) {
	return 0, nil
}
func (f *fakeSIMPowerBackend) CloseLogicalChannel(ctx context.Context, channelID int) error {
	return nil
}
func (f *fakeSIMPowerBackend) TransmitAPDU(ctx context.Context, channelID int, command string) (string, error) {
	return "", nil
}
func (f *fakeSIMPowerBackend) TransmitBasicAPDU(ctx context.Context, command string) (string, error) {
	return "", nil
}
func (f *fakeSIMPowerBackend) Mode() string {
	if f.mode != "" {
		return f.mode
	}
	return backend.BackendQMI
}
func (f *fakeSIMPowerBackend) Close() error { return nil }
func (f *fakeSIMPowerBackend) UIMPowerOffSIM(ctx context.Context, slot uint8) error {
	f.powerOffSlots = append(f.powerOffSlots, slot)
	return f.powerOffErr
}
func (f *fakeSIMPowerBackend) UIMPowerOnSIM(ctx context.Context, slot uint8) error {
	f.powerOnSlots = append(f.powerOnSlots, slot)
	return f.powerOnErr
}

func mustTestICCID(t *testing.T, value string) sgp22.ICCID {
	t.Helper()
	iccid, err := sgp22.NewICCID(value)
	if err != nil {
		t.Fatalf("NewICCID(%q) error=%v", value, err)
	}
	return iccid
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	out, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("DecodeString(%q) error=%v", value, err)
	}
	return out
}

func TestEUICCSpecConstantsAreStable(t *testing.T) {
	if EUICCSpecSGP22 != "sgp22" {
		t.Fatalf("EUICCSpecSGP22=%q want sgp22", EUICCSpecSGP22)
	}
	if EUICCSpecSGP32 != "sgp32" {
		t.Fatalf("EUICCSpecSGP32=%q want sgp32", EUICCSpecSGP32)
	}
	if EUICCSpecSGP02 != "sgp02" {
		t.Fatalf("EUICCSpecSGP02=%q want sgp02", EUICCSpecSGP02)
	}
}

func TestBuildEUICCInfoMarksLPACompatibleWithoutStrictSpec(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x05, 0x59, 0x10, 0x10, 0xFF, 0xFF, 0xFF, 0xFF, 0x89, 0x00, 0x00, 0x01, 0x00}

	info := buildDiscoveredEUICCInfo(aid, "eid-1")

	if info.Spec != EUICCSpecUnknown {
		t.Fatalf("Spec=%q want empty strict spec", info.Spec)
	}
	if info.SpecGuess != euiccSpecGuessSGP22Compat {
		t.Fatalf("SpecGuess=%q want %q", info.SpecGuess, euiccSpecGuessSGP22Compat)
	}
	if info.SpecConfidence != euiccSpecConfidenceInferred {
		t.Fatalf("SpecConfidence=%q want %q", info.SpecConfidence, euiccSpecConfidenceInferred)
	}
	if !reflect.DeepEqual(info.AID, aid) {
		t.Fatalf("AID=%X want %X", info.AID, aid)
	}
	if info.AIDHex != "A0000005591010FFFFFFFF8900000100" {
		t.Fatalf("AIDHex=%q want A0000005591010FFFFFFFF8900000100", info.AIDHex)
	}
	if info.EID != "eid-1" {
		t.Fatalf("EID=%q want eid-1", info.EID)
	}
}

func mustHexAIDs(t *testing.T, values ...string) [][]byte {
	t.Helper()
	out := make([][]byte, 0, len(values))
	for _, value := range values {
		out = append(out, mustDecodeHex(t, value))
	}
	return out
}

func aidHexList(aids [][]byte) []string {
	out := make([]string, 0, len(aids))
	for _, aid := range aids {
		out = append(out, strings.ToUpper(hex.EncodeToString(aid)))
	}
	return out
}

func TestGetEffectiveAIDPlanUsesFullStaticWithoutKnownAIDs(t *testing.T) {
	mgr := newManagerWithChannelFactory("dev-esim", nil, nil, nil, nil)

	plan := mgr.getEffectiveAIDPlan()

	if plan.Policy != aidScanPolicyFullStatic {
		t.Fatalf("Policy=%q want %q", plan.Policy, aidScanPolicyFullStatic)
	}
	if got, want := aidHexList(plan.AIDs), aidHexList(AIDs); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan AIDs=%v want full static AIDs %v", got, want)
	}
}

func TestGetEffectiveAIDPlanIgnoresSeededAIDs(t *testing.T) {
	seeded := mustHexAIDs(t, "A0000005591010FFFFFFFF8900000101")
	mgr := newManagerWithChannelFactory("dev-esim", nil, nil, nil, nil)
	mgr.SeedDiscoveredEUICCs([]EUICCInfo{{AID: seeded[0], EID: "eid-1", Spec: EUICCSpecSGP22}})

	plan := mgr.getEffectiveAIDPlan()

	if plan.Policy != aidScanPolicyFullStatic {
		t.Fatalf("Policy=%q want %q", plan.Policy, aidScanPolicyFullStatic)
	}
	if got, want := aidHexList(plan.AIDs), aidHexList(AIDs); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan AIDs=%v want full static AIDs %v", got, want)
	}
}

func TestGetEffectiveAIDsClonesPlanAIDs(t *testing.T) {
	mgr := newManagerWithChannelFactory("dev-esim", nil, nil, nil, nil)

	got := mgr.getEffectiveAIDs()
	got[0][0] = 0xFF

	if bytes.Equal(got[0], AIDs[0]) {
		t.Fatal("getEffectiveAIDs returned static AIDs backing slice; want clone")
	}
}

func TestSeedDiscoveredEUICCsDoesNotChangeScanPlan(t *testing.T) {
	aid := mustHexAIDs(t, "A0000005591010FFFFFFFF8900000199")[0]
	mgr := newManagerWithChannelFactory("dev-esim", nil, nil, nil, nil)

	mgr.SeedDiscoveredEUICCs([]EUICCInfo{{AID: aid, EID: "eid-1", Spec: EUICCSpecSGP22}})

	plan := mgr.getEffectiveAIDPlan()
	if plan.Policy != aidScanPolicyFullStatic {
		t.Fatalf("Policy=%q want %q after seeding discovered eUICC", plan.Policy, aidScanPolicyFullStatic)
	}
	if got, want := aidHexList(plan.AIDs), aidHexList(AIDs); !reflect.DeepEqual(got, want) {
		t.Fatalf("plan AIDs=%v want full static AIDs %v", got, want)
	}
}

func TestGetEIDsScansHardwareEvenWhenSeededDiscoveryExists(t *testing.T) {
	seededAID := mustHexAIDs(t, "A0000005591010FFFFFFFF8900000199")[0]
	targetAID := AIDs[2]
	targetAIDHex := strings.ToUpper(hex.EncodeToString(targetAID))
	targetEID := "89049032000001000000113509931049"
	var factoryCalls atomic.Int32
	mgr := newManagerWithChannelFactory("reader-slot", func(aid []byte) (*lpa.Client, error) {
		factoryCalls.Add(1)
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != targetAIDHex {
			return nil, fmt.Errorf("unsupported AID %s", got)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{eid: mustDecodeHex(t, targetEID)}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }
	mgr.SeedDiscoveredEUICCs([]EUICCInfo{{
		AID:    seededAID,
		AIDHex: "A0000005591010FFFFFFFF8900000199",
		EID:    "89049032000001000000113509931048",
		Spec:   EUICCSpecSGP22,
	}})

	eids, err := mgr.GetEIDs()
	if err != nil {
		t.Fatalf("GetEIDs() error=%v", err)
	}
	if factoryCalls.Load() == 0 {
		t.Fatal("channel factory calls=0, want hardware scan despite seeded discovery")
	}
	if len(eids) != 1 {
		t.Fatalf("len(GetEIDs())=%d want 1", len(eids))
	}
	if eids[0].EID != targetEID || eids[0].AIDHex != targetAIDHex {
		t.Fatalf("GetEIDs()=%#v, want scanned AID/EID", eids)
	}
}

func TestGetEsimOverviewSeedsDiscoveredEIDForLaterGetEIDs(t *testing.T) {
	aid := mustHexAIDs(t, "A0000005591010FFFFFFFF8900000100")[0]
	aidHex := strings.ToUpper(hex.EncodeToString(aid))
	var factoryCalls atomic.Int32
	eid := mustHexAIDs(t, "89049032000001000000113509931049")[0]
	mgr := newManagerWithChannelFactory("reader-slot", func(aid []byte) (*lpa.Client, error) {
		factoryCalls.Add(1)
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != aidHex {
			return nil, fmt.Errorf("unexpected AID %s", got)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{eid: eid}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	overview, err := mgr.GetEsimOverview()
	if err != nil {
		t.Fatalf("GetEsimOverview() error=%v", err)
	}
	if overview == nil || overview.ChipInfo == nil || len(overview.ChipInfo.EIDs) != 1 {
		t.Fatalf("overview=%#v, want one EID", overview)
	}
	callsAfterOverview := factoryCalls.Load()

	eids, err := mgr.GetEIDs()
	if err != nil {
		t.Fatalf("GetEIDs() after overview error=%v", err)
	}
	if factoryCalls.Load() <= callsAfterOverview {
		t.Fatalf("channel factory calls after GetEIDs=%d want more than %d for fresh scan", factoryCalls.Load(), callsAfterOverview)
	}
	if len(eids) != 1 || eids[0].EID != "89049032000001000000113509931049" || eids[0].AIDHex != aidHex {
		t.Fatalf("GetEIDs()=%#v, want seeded overview AID/EID", eids)
	}
}

func TestForEachEUICCAlwaysUsesFullStaticScan(t *testing.T) {
	targetAID := cloneAIDList([][]byte{AIDs[3]})[0]
	targetHex := strings.ToUpper(hex.EncodeToString(targetAID))
	attempts := make(map[string]int)
	seenAttempts := make([]string, 0, len(AIDs))

	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		attempts[aidHex]++
		seenAttempts = append(seenAttempts, aidHex)
		if aidHex != targetHex {
			return nil, fmt.Errorf("static AID should fail: %s", aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{
			eid: []byte{0x89, 0x04, 0x40, 0x45, 0x84, 0x67, 0x27, 0x49},
		}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	var callbackAID string
	err := mgr.forEachEUICC(func(client *lpa.Client, aid []byte, eidStr string) error {
		callbackAID = strings.ToUpper(hex.EncodeToString(aid))
		return nil
	})

	if err != nil {
		t.Fatalf("forEachEUICC() error=%v, attempts=%v", err, seenAttempts)
	}
	if attempts[targetHex] != 1 {
		t.Fatalf("target AID attempts=%d want 1, all attempts=%v", attempts[targetHex], seenAttempts)
	}
	wantAttempts := aidHexList(AIDs[:4])
	if !reflect.DeepEqual(seenAttempts, wantAttempts) {
		t.Fatalf("attempted AIDs=%v want full scan %v", seenAttempts, wantAttempts)
	}
	if callbackAID != targetHex {
		t.Fatalf("callback AID=%s want %s", callbackAID, targetHex)
	}

	seenAttempts = nil
	callbackAID = ""
	err = mgr.forEachEUICC(func(client *lpa.Client, aid []byte, eidStr string) error {
		callbackAID = strings.ToUpper(hex.EncodeToString(aid))
		return nil
	})

	if err != nil {
		t.Fatalf("second forEachEUICC() error=%v, attempts=%v", err, seenAttempts)
	}
	if wantAttempts := aidHexList(AIDs[:4]); !reflect.DeepEqual(seenAttempts, wantAttempts) {
		t.Fatalf("second attempted AIDs=%v want fresh full scan %v", seenAttempts, wantAttempts)
	}
	if callbackAID != targetHex {
		t.Fatalf("second callback AID=%s want %s", callbackAID, targetHex)
	}
}

func TestDoForEachEUICCScansPastThreeFailuresAndStopsAtFirstUsableVendorAID(t *testing.T) {
	targetAIDHex := strings.ToUpper(hex.EncodeToString(AIDs[3]))
	eid := mustDecodeHex(t, "89049032000001000000113509931049")
	opened := make([]string, 0, len(AIDs))
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		opened = append(opened, aidHex)
		if aidHex != targetAIDHex {
			return nil, fmt.Errorf("unsupported AID %s", aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{eid: eid}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	var callbackAID string
	found, err := mgr.doForEachEUICC(AIDs, func(client *lpa.Client, aid []byte, eidStr string) error {
		callbackAID = strings.ToUpper(hex.EncodeToString(aid))
		return nil
	})

	if err != nil {
		t.Fatalf("doForEachEUICC() error=%v, opened=%v", err, opened)
	}
	if !found {
		t.Fatalf("doForEachEUICC() found=false, opened=%v", opened)
	}
	wantOpened := aidHexList(AIDs[:4])
	if !reflect.DeepEqual(opened, wantOpened) {
		t.Fatalf("opened AIDs=%v want %v", opened, wantOpened)
	}
	if callbackAID != targetAIDHex {
		t.Fatalf("callback AID=%s want %s", callbackAID, targetAIDHex)
	}
}

// TestDoForEachEUICCRecoversFromCallbackPanic guards against a real incident:
// a malformed GetProfilesInfo response (missing the profile-state TLV tag)
// makes the euicc-go v1.1.2 BER-TLV decoder nil-deref panic deep inside the
// per-AID callback. Without recovery, that panic unwinds past forEachEUICC
// and is only caught by gin's process-wide Recovery middleware, which throws
// away the already-discovered eUICC/profile data and reports a generic 500
// that the frontend renders as "未检测到 eUICC". The callback must be allowed
// to panic per-AID without taking down the whole overview/profile scan.
func TestDoForEachEUICCRecoversFromCallbackPanic(t *testing.T) {
	targetAIDHex := strings.ToUpper(hex.EncodeToString(AIDs[3]))
	eid := mustDecodeHex(t, "89049032000001000000113509931049")
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		if aidHex != targetAIDHex {
			return nil, fmt.Errorf("unsupported AID %s", aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{eid: eid}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	found, err := mgr.doForEachEUICC(AIDs, func(client *lpa.Client, aid []byte, eidStr string) error {
		var nilTLV *bertlv.TLV
		return nilTLV.UnmarshalValue(nil) // panics: nil pointer dereference, mirrors the upstream euicc-go bug
	})

	// found=true/err=nil matches the existing "already discovered an eUICC, so
	// don't fail the whole scan over one bad AID" contract (see the lastErr
	// swallow a few lines above in doForEachEUICC). The real assertion this
	// test guards is implicit: reaching this line at all means the panic was
	// recovered instead of crashing the test (and, in production, the request).
	if !found {
		t.Fatalf("doForEachEUICC() found=false, want true (eUICC was discovered before the callback panicked)")
	}
	if err != nil {
		t.Fatalf("doForEachEUICC() error=%v, want nil (recovered panic should be swallowed like any other per-AID failure once an eUICC was found)", err)
	}
}

func TestDoForEachEUICCContinuesESTKPairThenStopsBeforeGSMA(t *testing.T) {
	eids := map[string][]byte{
		strings.ToUpper(hex.EncodeToString(AIDs[0])): mustDecodeHex(t, "89049032000001000000113509931049"),
		strings.ToUpper(hex.EncodeToString(AIDs[1])): mustDecodeHex(t, "89049032000001000000113509931050"),
	}
	opened := make([]string, 0, len(AIDs))
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		opened = append(opened, aidHex)
		eid, ok := eids[aidHex]
		if !ok {
			return nil, fmt.Errorf("should stop before AID %s", aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{eid: eid}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	found, err := mgr.doForEachEUICC(AIDs, func(client *lpa.Client, aid []byte, eidStr string) error {
		return nil
	})

	if err != nil {
		t.Fatalf("doForEachEUICC() error=%v, opened=%v", err, opened)
	}
	if !found {
		t.Fatalf("doForEachEUICC() found=false, opened=%v", opened)
	}
	wantOpened := aidHexList(AIDs[:2])
	if !reflect.DeepEqual(opened, wantOpened) {
		t.Fatalf("opened AIDs=%v want only eSTK SE0/SE1 %v", opened, wantOpened)
	}
}

func TestFindAIDForICCIDScansPastThreeFailedAIDs(t *testing.T) {
	targetICCID := "8986001234567890123"
	targetAID := AIDs[3]
	opened := make([]string, 0, len(AIDs))
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		opened = append(opened, aidHex)
		if !reflect.DeepEqual(aid, targetAID) {
			return nil, fmt.Errorf("unsupported AID %s", aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{
			profiles: []*sgp22.ProfileInfo{{ICCID: mustTestICCID(t, targetICCID)}},
		}}, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	got, err := mgr.findAIDForICCID(targetICCID)
	if err != nil {
		t.Fatalf("findAIDForICCID() error=%v opened=%v", err, opened)
	}
	if !reflect.DeepEqual(got, targetAID) {
		t.Fatalf("findAIDForICCID()=%X want %X", got, targetAID)
	}
	if want := aidHexList(AIDs[:4]); !reflect.DeepEqual(opened, want) {
		t.Fatalf("opened=%v want full scan %v", opened, want)
	}
}

func newTestQMIManagerForPowerCycle(t *testing.T, be *fakeSIMPowerBackend, calls *atomic.Int32) *Manager {
	t.Helper()
	return &Manager{
		deviceID:  "dev-esim",
		transport: transportQMI,
		backend:   be,
		channelFactory: func(aid []byte) (*lpa.Client, error) {
			return &lpa.Client{APDU: fakeProfileOperationTransmitter{calls: calls}}, nil
		},
		closeClient:  func(client *lpa.Client) error { return nil },
		switchSignal: make(chan string, 16),
		opDone:       make(chan struct{}),
		sf:           &singleflight.Group{},
	}
}

// newTestATManagerForSIMReload 创建一个用于测试 AT 通道 SIM 重载的 Manager 实例，指定后端模式为 AT
func newTestATManagerForSIMReload(t *testing.T, be *fakeSIMPowerBackend, calls *atomic.Int32) *Manager {
	t.Helper()
	be.mode = backend.BackendAT
	return &Manager{
		deviceID:  "dev-esim",
		transport: transportAT,
		backend:   be,
		channelFactory: func(aid []byte) (*lpa.Client, error) {
			return &lpa.Client{APDU: fakeProfileOperationTransmitter{calls: calls}}, nil
		},
		closeClient:  func(client *lpa.Client) error { return nil },
		switchSignal: make(chan string, 16),
		opDone:       make(chan struct{}),
		sf:           &singleflight.Group{},
	}
}

func TestSwitchProfileQMIDoesNotPowerCycleAfterEnableSuccess(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	if err := mgr.SwitchProfile(context.Background(), targetICCID, aidHex); err != nil {
		t.Fatalf("SwitchProfile() error=%v", err)
	}
	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}

	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran SIM reload power cycle: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
}

func TestSwitchProfileATDoesNotReloadInsideManagerAfterEnableSuccess(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{}
	mgr := newTestATManagerForSIMReload(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}

	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if len(be.setModeCalls) != 0 {
		t.Fatalf("Manager ran AT SIM reload: setModeCalls=%v", be.setModeCalls)
	}
	if result.PowerCycleAttempt {
		t.Fatal("PowerCycleAttempt=true before async recovery, want false")
	}
	if result.SIMReloadWarning != "" {
		t.Fatalf("SIMReloadWarning=%q want empty", result.SIMReloadWarning)
	}
}

func TestSwitchProfileDoesNotReportReloadWarningFromManager(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	originalTimeout := switchFallbackPowerTimeout
	switchFallbackPowerTimeout = time.Millisecond
	t.Cleanup(func() { switchFallbackPowerTimeout = originalTimeout })

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{powerOffErr: errors.New("power off failed")}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if !result.PostSwitchAsync {
		t.Fatal("PostSwitchAsync should remain true after switch command is accepted")
	}
	if !result.SwitchAccepted {
		t.Fatal("SwitchAccepted should be true after EnableProfile is accepted")
	}
	if !result.RecoveryPending {
		t.Fatal("RecoveryPending should be true when post-switch recovery is scheduled")
	}
	if result.SIMReloadWarning != "" || result.DegradedReason != "" {
		t.Fatalf("result=%#v should not report async reload warning before return", result)
	}
	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran SIM reload despite backend power failure setup: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
}

func TestSwitchProfileDoesNotProbeIdentityInsideManagerAfterAcceptedSwitch(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{iccid: targetICCID}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if result.PowerCycleAttempt {
		t.Fatal("PowerCycleAttempt=true, want false when target identity is already readable")
	}
	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}
	if got := len(be.powerOffSlots); got != 0 {
		t.Fatalf("powerOff calls=%d want 0", got)
	}
	if got := len(be.powerOnSlots); got != 0 {
		t.Fatalf("powerOn calls=%d want 0", got)
	}
	if be.iccidCalls.Load() != 0 {
		t.Fatalf("GetICCIDLive calls=%d want 0; Pool owns post-switch identity convergence", be.iccidCalls.Load())
	}
}

func TestSwitchProfileDoesNotRunSIMReloadInsideManager(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{iccid: "8986000000000000000"}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if result.PowerCycleAttempt {
		t.Fatal("PowerCycleAttempt=true before async recovery, want false")
	}
	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran SIM reload power cycle: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
	if be.iccidCalls.Load() != 0 {
		t.Fatalf("GetICCIDLive calls=%d want 0; Pool owns post-switch identity convergence", be.iccidCalls.Load())
	}
}

func TestSwitchProfilePostSwitchHookRunsWithoutManagerReload(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{iccid: "8986000000000000000"}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = 100 * time.Millisecond
	hookDone := make(chan struct{}, 1)
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- struct{}{}
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	if !result.SwitchAccepted || !result.PostSwitchAsync || !result.RecoveryPending {
		t.Fatalf("result=%#v want accepted async recovery", result)
	}
	if result.PowerCycleAttempt {
		t.Fatal("PowerCycleAttempt=true before async recovery, want false")
	}
	if got := len(be.powerOffSlots); got != 0 {
		t.Fatalf("powerOff calls before return=%d want 0", got)
	}
	if got := len(be.powerOnSlots); got != 0 {
		t.Fatalf("powerOn calls before return=%d want 0", got)
	}

	select {
	case <-hookDone:
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran async SIM reload power cycle: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
}

func TestSwitchProfileSuppressesOverviewReloadBeforeEnableRefresh(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{iccid: targetICCID}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.channelFactory = func(aid []byte) (*lpa.Client, error) {
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{
			calls: &enableCalls,
			onProfileOperation: func() {
				if !mgr.shouldSuppressOverviewReload() {
					t.Fatal("overview reload suppression is not active before EnableProfile refresh")
				}
			},
		}}, nil
	}

	if _, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex); err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
}

func TestDisableProfileDoesNotPowerCycleInsideManager(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var disableCalls atomic.Int32
	be := &fakeSIMPowerBackend{}
	mgr := newTestQMIManagerForPowerCycle(t, be, &disableCalls)
	mgr.postSwitchMinDelay = 0

	if err := mgr.DisableProfile(context.Background(), targetICCID, aidHex); err != nil {
		t.Fatalf("DisableProfile() error=%v", err)
	}

	if disableCalls.Load() != 1 {
		t.Fatalf("DisableProfile calls=%d want 1", disableCalls.Load())
	}
	time.Sleep(20 * time.Millisecond)
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran SIM reload power cycle after disable: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
}

func TestDisableProfileTreatsMBIMInvalidChannelAsExpected(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var disableCalls atomic.Int32
	be := &fakeSIMPowerBackend{}
	mgr := &Manager{
		deviceID:  "dev-mbim",
		transport: transportMBIM,
		backend:   be,
		channelFactory: func(aid []byte) (*lpa.Client, error) {
			return &lpa.Client{APDU: fakeProfileOperationTransmitter{
				calls: &disableCalls,
				err:   fmt.Errorf("transmit APDU: %w", ErrMBIMUICCInvalidChannel),
			}}, nil
		},
		closeClient:  func(client *lpa.Client) error { return nil },
		switchSignal: make(chan string, 16),
		opDone:       make(chan struct{}),
		sf:           &singleflight.Group{},
	}
	mgr.postSwitchMinDelay = 0

	if err := mgr.DisableProfile(context.Background(), targetICCID, aidHex); err != nil {
		t.Fatalf("DisableProfile() error=%v, want nil (MBIM逻辑通道失效是 refresh 后预期信号)", err)
	}
	if disableCalls.Load() != 1 {
		t.Fatalf("DisableProfile calls=%d want 1", disableCalls.Load())
	}
}

func TestSwitchProfileCustomAPDUErrorFailsWithoutConfirmationRead(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "894921007618265679f"
	var enableCalls atomic.Int32
	var listCalls atomic.Int32
	eid, err := hex.DecodeString("89049032000001000000113509931049")
	if err != nil {
		t.Fatal(err)
	}

	transmitter := fakeProfileOperationTransmitter{
		err:       errors.New("returned an unexpected response with status 910B"),
		calls:     &enableCalls,
		listCalls: &listCalls,
		eid:       eid,
		profiles: []*sgp22.ProfileInfo{
			{ICCID: mustTestICCID(t, "989412006781626597ff"), ProfileState: sgp22.ProfileDisabled, ProfileName: "old"},
			{ICCID: mustTestICCID(t, targetICCID), ProfileState: sgp22.ProfileEnabled, ProfileName: "target"},
		},
	}
	mgr := newManagerWithChannelFactory("reader-slot", func(aid []byte) (*lpa.Client, error) {
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != aidHex {
			t.Fatalf("aid=%s want %s", got, aidHex)
		}
		return &lpa.Client{APDU: transmitter}, nil
	}, nil, nil, nil)
	mgr.profilesLoader = func() ([]EUICCProfiles, error) {
		if !mgr.opMu.TryLock() {
			return nil, ErrOperationInProgress
		}
		mgr.opMu.Unlock()
		return nil, errors.New("RefreshProfiles should not be used during locked switch confirmation")
	}
	mgr.postSwitchMinDelay = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = mgr.SwitchProfile(ctx, targetICCID, aidHex)
	if err == nil {
		t.Fatal("SwitchProfile() error=nil, want EnableProfile APDU error")
	}
	if !strings.Contains(err.Error(), "returned an unexpected response with status 910B") {
		t.Fatalf("SwitchProfile() error=%v, want original APDU status", err)
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if listCalls.Load() != 0 {
		t.Fatalf("profile list calls=%d want 0; post-read confirmation must not change switch result", listCalls.Load())
	}
}

func TestSwitchProfileQMIErrorFailsEvenWhenBackendReportsTargetICCID(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{iccid: targetICCID}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.channelFactory = func(aid []byte) (*lpa.Client, error) {
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != aidHex {
			t.Fatalf("aid=%s want %s", got, aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{
			err: &qmiq.QMIError{
				Service:   qmiq.ServiceUIM,
				Result:    1,
				ErrorCode: qmiq.QMIErrDeviceNotReady,
			},
			calls: &enableCalls,
		}}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	result, err := mgr.SwitchProfileWithResult(ctx, targetICCID, aidHex)
	if err == nil {
		t.Fatal("SwitchProfileWithResult() error=nil, want QMI EnableProfile error")
	}
	if !strings.Contains(err.Error(), "error=0x0005") {
		t.Fatalf("SwitchProfileWithResult() error=%v, want original QMI error code", err)
	}
	if result.SwitchAccepted {
		t.Fatal("SwitchAccepted=true, want false for pre-acceptance QMI error")
	}
	if enableCalls.Load() != 1 {
		t.Fatalf("EnableProfile calls=%d want 1", enableCalls.Load())
	}
	if be.iccidCalls.Load() != 0 {
		t.Fatalf("GetICCID calls=%d want 0; backend state must not redefine switch command success", be.iccidCalls.Load())
	}
}

func TestSwitchProfileFailedCallbackReceivesOriginalError(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	var enableCalls atomic.Int32
	rootErr := errors.New("QMI: read failed: EOF")
	mgr := newTestQMIManagerForPowerCycle(t, &fakeSIMPowerBackend{}, &enableCalls)
	mgr.channelFactory = func(aid []byte) (*lpa.Client, error) {
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != aidHex {
			t.Fatalf("aid=%s want %s", got, aidHex)
		}
		return &lpa.Client{APDU: fakeProfileOperationTransmitter{
			err:   rootErr,
			calls: &enableCalls,
		}}, nil
	}
	mgr.onBeforeSwitch = func(operation SwitchOperation, target string) uint64 {
		if operation != SwitchOperationEnableProfile {
			t.Fatalf("operation=%q want %q", operation, SwitchOperationEnableProfile)
		}
		if target != targetICCID {
			t.Fatalf("target=%q want %q", target, targetICCID)
		}
		return 42
	}
	var callbackErr error
	var callbackToken uint64
	mgr.onSwitchFailed = func(operation SwitchOperation, token uint64, err error) {
		if operation != SwitchOperationEnableProfile {
			t.Fatalf("operation=%q want %q", operation, SwitchOperationEnableProfile)
		}
		callbackToken = token
		callbackErr = err
	}

	_, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err == nil {
		t.Fatal("SwitchProfileWithResult() error=nil, want QMI error")
	}
	if callbackToken != 42 {
		t.Fatalf("callback token=%d want 42", callbackToken)
	}
	if !errors.Is(callbackErr, rootErr) {
		t.Fatalf("callback error=%v want wrapping root %v", callbackErr, rootErr)
	}
}

func TestSwitchProfilePostSwitchHookStillRunsWhenBackendPowerWouldFail(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	const targetICCID = "8986001234567890123"
	originalTimeout := switchFallbackPowerTimeout
	switchFallbackPowerTimeout = time.Millisecond
	t.Cleanup(func() { switchFallbackPowerTimeout = originalTimeout })

	var enableCalls atomic.Int32
	be := &fakeSIMPowerBackend{powerOffErr: errors.New("power off failed")}
	mgr := newTestQMIManagerForPowerCycle(t, be, &enableCalls)
	mgr.postSwitchMinDelay = time.Millisecond
	hookDone := make(chan uint64, 1)
	mgr.onBeforeSwitch = func(operation SwitchOperation, target string) uint64 {
		return 12
	}
	mgr.onAfterSwitch = func(operation SwitchOperation, token uint64) {
		hookDone <- token
	}

	result, err := mgr.SwitchProfileWithResult(context.Background(), targetICCID, aidHex)
	if err != nil {
		t.Fatalf("SwitchProfileWithResult() error=%v", err)
	}
	if !result.SwitchAccepted {
		t.Fatal("SwitchAccepted=false, want true")
	}
	if !result.PostSwitchAsync {
		t.Fatal("PostSwitchAsync=false, want true")
	}
	select {
	case token := <-hookDone:
		if token != 12 {
			t.Fatalf("post-switch hook token=%d want 12", token)
		}
	case <-time.After(time.Second):
		t.Fatal("post-switch hook did not run")
	}
	if len(be.powerOffSlots) != 0 || len(be.powerOnSlots) != 0 {
		t.Fatalf("Manager ran SIM reload despite backend power failure setup: off=%v on=%v", be.powerOffSlots, be.powerOnSlots)
	}
}

func TestRunPostSwitchHookStillCallsAfterSwitchWhenAPDUStaysBusy(t *testing.T) {
	var afterCalls atomic.Int32
	mgr := &Manager{
		deviceID:           "dev-esim",
		onAfterSwitch:      func(SwitchOperation, uint64) { afterCalls.Add(1) },
		apduArbiter:        fakeAPDUIdleWaiter{wait: func(ctx context.Context) error { return context.DeadlineExceeded }},
		postSwitchMinDelay: time.Millisecond,
	}

	mgr.runPostSwitchHook(SwitchOperationEnableProfile, 0)

	// APDU 忙时仍应执行 onAfterSwitch 回调，避免切卡后上层恢复流程被跳过。
	if afterCalls.Load() != 1 {
		t.Fatalf("onAfterSwitch calls=%d want 1", afterCalls.Load())
	}
}

func TestRunPostSwitchHookCallsAfterSwitchWhenAPDUBecomesIdle(t *testing.T) {
	var afterCalls atomic.Int32
	mgr := &Manager{
		deviceID:           "dev-esim",
		onAfterSwitch:      func(SwitchOperation, uint64) { afterCalls.Add(1) },
		apduArbiter:        fakeAPDUIdleWaiter{wait: func(ctx context.Context) error { return nil }},
		postSwitchMinDelay: time.Millisecond,
	}

	mgr.runPostSwitchHook(SwitchOperationEnableProfile, 0)

	if afterCalls.Load() != 1 {
		t.Fatalf("onAfterSwitch calls=%d want 1", afterCalls.Load())
	}
}

func TestRunPostSwitchHookPassesSwitchOperation(t *testing.T) {
	var got SwitchOperation
	mgr := &Manager{
		deviceID:           "dev-esim",
		onAfterSwitch:      func(op SwitchOperation, token uint64) { got = op },
		postSwitchMinDelay: time.Millisecond,
	}

	mgr.runPostSwitchHook(SwitchOperationDisableProfile, 7)

	if got != SwitchOperationDisableProfile {
		t.Fatalf("operation=%q want %q", got, SwitchOperationDisableProfile)
	}
}

func TestNewManagerWithChannelFactoryUsesOneSecondDefaultPostSwitchDelay(t *testing.T) {
	mgr := newManagerWithChannelFactory("dev-esim", nil, nil, nil, nil)
	if mgr.postSwitchMinDelay != time.Second {
		t.Fatalf("postSwitchMinDelay=%s want 1s", mgr.postSwitchMinDelay)
	}
}

func TestNewManagerMBIMTransportUsesQMIChannelOverProvidedTransport(t *testing.T) {
	mgr, err := NewManager(ManagerOptions{
		DeviceID:     "dev-mbim",
		Transport:    transportMBIM,
		Backend:      &fakeSIMPowerBackend{},
		QMITransport: &qmiChannelTransportFake{controlDevice: "/dev/cdc-wdm2"},
	})
	if err != nil {
		t.Fatalf("NewManager(mbim) error = %v", err)
	}
	if mgr.transport != transportMBIM {
		t.Fatalf("transport=%q want %q", mgr.transport, transportMBIM)
	}
	if mgr.controlDevice != "/dev/cdc-wdm2" {
		t.Fatalf("controlDevice=%q want /dev/cdc-wdm2", mgr.controlDevice)
	}
	ch, err := mgr.newSmartCardChannel()
	if err != nil {
		t.Fatalf("newSmartCardChannel error = %v", err)
	}
	if _, ok := ch.(*QMIChannel); !ok {
		t.Fatalf("mbim transport should build *QMIChannel over the APDU transport, got %T", ch)
	}
}

func TestNewManagerMBIMTransportRequiresTransport(t *testing.T) {
	_, err := NewManager(ManagerOptions{
		DeviceID:  "dev-mbim",
		Transport: transportMBIM,
		Backend:   &fakeSIMPowerBackend{},
	})
	if !errors.Is(err, ErrQMITransportNotAvailable) {
		t.Fatalf("NewManager(mbim, nil transport) error = %v, want ErrQMITransportNotAvailable", err)
	}
}

func TestNewManagerCustomTransportUsesProvidedSmartCardChannelFactory(t *testing.T) {
	want := &lifecycleSmartCardChannel{}
	mgr, err := NewManager(ManagerOptions{
		DeviceID:  "dev-custom",
		Transport: transportCustom,
		SmartCardChannelFactory: func() (driver.SmartCardChannel, error) {
			return want, nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager(custom) error = %v", err)
	}
	got, err := mgr.newSmartCardChannel()
	if err != nil {
		t.Fatalf("newSmartCardChannel error = %v", err)
	}
	if got != want {
		t.Fatalf("newSmartCardChannel returned %T %p, want %p", got, got, want)
	}
}

func TestNewManagerCustomTransportRequiresSmartCardChannelFactory(t *testing.T) {
	_, err := NewManager(ManagerOptions{
		DeviceID:  "dev-custom",
		Transport: transportCustom,
	})
	if err == nil || !strings.Contains(err.Error(), "smart card channel factory") {
		t.Fatalf("NewManager(custom without factory) error = %v, want factory requirement", err)
	}
}

func TestNewManagerWithoutAPDUArbiterLeavesReadIdleWaitDisabled(t *testing.T) {
	mgr, err := NewManager(ManagerOptions{
		DeviceID:  "dev-custom",
		Transport: transportCustom,
		SmartCardChannelFactory: func() (driver.SmartCardChannel, error) {
			return &lifecycleSmartCardChannel{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager(custom) error = %v", err)
	}
	if mgr.apduArbiter != nil {
		t.Fatalf("apduArbiter=%#v, want nil", mgr.apduArbiter)
	}
	if err := mgr.waitForAPDUIdleForRead(); err != nil {
		t.Fatalf("waitForAPDUIdleForRead() error = %v, want nil", err)
	}
}

func TestFinalizeEnableProfileResultTreatsMBIMInvalidChannelAsExpected(t *testing.T) {
	m := &Manager{deviceID: "dev-mbim"}
	wrapped := fmt.Errorf("transmit APDU: %w", ErrMBIMUICCInvalidChannel)
	if err := m.finalizeEnableProfileResult("8986001234567890123", wrapped); err != nil {
		t.Fatalf("finalizeEnableProfileResult() error = %v, want nil (MBIM逻辑通道失效是 refresh 后预期信号)", err)
	}
}

func TestFinalizeEnableProfileResultStillFailsOnUnrelatedMBIMError(t *testing.T) {
	m := &Manager{deviceID: "dev-mbim"}
	wrapped := fmt.Errorf("transmit APDU: %w", errors.New("some other failure"))
	if err := m.finalizeEnableProfileResult("8986001234567890123", wrapped); err == nil {
		t.Fatal("finalizeEnableProfileResult() error = nil, want failure for unrelated error")
	}
}

func TestNewManagerKeepsExplicitPostSwitchDelayOverride(t *testing.T) {
	mgr, err := NewManager(ManagerOptions{
		DeviceID:           "dev-esim",
		Transport:          transportQMI,
		Backend:            &fakeSIMPowerBackend{},
		QMITransport:       &qmiChannelTransportFake{},
		PostSwitchMinDelay: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if mgr.postSwitchMinDelay != 3*time.Second {
		t.Fatalf("postSwitchMinDelay=%s want 3s", mgr.postSwitchMinDelay)
	}
}

func TestNewManagerConfiguresSwitchRefreshFlag(t *testing.T) {
	defaultMgr, err := NewManager(ManagerOptions{
		DeviceID:     "dev-esim",
		Transport:    transportQMI,
		Backend:      &fakeSIMPowerBackend{},
		QMITransport: &qmiChannelTransportFake{},
	})
	if err != nil {
		t.Fatalf("NewManager(default) error = %v", err)
	}
	if defaultMgr.switchUseRefreshTrue {
		t.Fatal("switchUseRefreshTrue=true by default, want false")
	}

	refreshMgr, err := NewManager(ManagerOptions{
		DeviceID:             "dev-esim",
		Transport:            transportQMI,
		Backend:              &fakeSIMPowerBackend{},
		QMITransport:         &qmiChannelTransportFake{},
		SwitchUseRefreshTrue: true,
	})
	if err != nil {
		t.Fatalf("NewManager(refresh true) error = %v", err)
	}
	if !refreshMgr.switchUseRefreshTrue {
		t.Fatal("switchUseRefreshTrue=false, want true")
	}
}

func TestGetEsimOverviewCachesSuccessfulLoad(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{SkuName: "cached-chip"},
			Profiles: []EUICCProfiles{{EID: "eid-1", AIDHex: "A000", Profiles: []ProfileItem{{ICCID: "iccid-1", Name: "profile-1"}}}},
		}, nil
	})

	got1, err := mgr.GetEsimOverview()
	if err != nil {
		t.Fatalf("GetEsimOverview() first call error = %v", err)
	}
	got1.ChipInfo.SkuName = "mutated"
	got1.Profiles[0].Profiles[0].Name = "mutated"

	got2, err := mgr.GetEsimOverview()
	if err != nil {
		t.Fatalf("GetEsimOverview() second call error = %v", err)
	}

	if got2.ChipInfo.SkuName != "cached-chip" {
		t.Fatalf("GetEsimOverview() chip_info.sku = %q, want %q", got2.ChipInfo.SkuName, "cached-chip")
	}
	if got2.Profiles[0].Profiles[0].Name != "profile-1" {
		t.Fatalf("GetEsimOverview() profile name = %q, want %q", got2.Profiles[0].Profiles[0].Name, "profile-1")
	}
	if calls.Load() != 1 {
		t.Fatalf("overview loader calls = %d, want 1", calls.Load())
	}
}

func TestGetProfilesUsesCachedOverview(t *testing.T) {
	var calls atomic.Int32
	wantProfiles := []EUICCProfiles{{
		EID:    "eid-1",
		AIDHex: "A000",
		Profiles: []ProfileItem{{
			ICCID: "iccid-1",
			Name:  "profile-1",
		}},
	}}
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{SkuName: "chip"},
			Profiles: wantProfiles,
		}, nil
	})

	if _, err := mgr.GetEsimOverview(); err != nil {
		t.Fatalf("GetEsimOverview() error = %v", err)
	}

	got, err := mgr.GetProfiles()
	if err != nil {
		t.Fatalf("GetProfiles() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("overview loader calls = %d, want 1", calls.Load())
	}
	if len(got) != 1 || got[0].Profiles[0].ICCID != "iccid-1" {
		t.Fatalf("GetProfiles() = %#v, want cached profiles %#v", got, wantProfiles)
	}
}

func TestInvalidateOverviewCacheDropsStaleData(t *testing.T) {
	reloadErr := errors.New("reload failed")
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		if calls.Add(1) == 1 {
			return &EsimOverview{
				ChipInfo: &EUICCChipInfo{SkuName: "before"},
				Profiles: []EUICCProfiles{{EID: "eid-1", AIDHex: "A000", Profiles: []ProfileItem{{ICCID: "iccid-1"}}}},
			}, nil
		}
		return nil, reloadErr
	})

	if _, err := mgr.GetEsimOverview(); err != nil {
		t.Fatalf("GetEsimOverview() initial load error = %v", err)
	}

	mgr.invalidateOverviewCache("test")

	if _, err := mgr.GetProfiles(); !errors.Is(err, reloadErr) {
		t.Fatalf("GetProfiles() error = %v, want %v after invalidation", err, reloadErr)
	}
	if calls.Load() != 2 {
		t.Fatalf("overview loader calls = %d, want 2", calls.Load())
	}
}

func TestNotifyModemResetClearsOverviewAndDiscoveredEUICCs(t *testing.T) {
	reloadErr := errors.New("reload after reset failed")
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		if calls.Add(1) == 1 {
			return &EsimOverview{
				ChipInfo: &EUICCChipInfo{SkuName: "before-reset"},
				Profiles: []EUICCProfiles{{EID: "eid-1", AIDHex: "A000", Profiles: []ProfileItem{{ICCID: "iccid-1"}}}},
			}, nil
		}
		return nil, reloadErr
	})
	mgr.discoveredEUICCs = []EUICCInfo{{AIDHex: "0102", EID: "eid-before-reset", Spec: EUICCSpecSGP22}}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "before-reset"}

	if _, err := mgr.GetEsimOverview(); err != nil {
		t.Fatalf("GetEsimOverview() initial load error = %v", err)
	}

	mgr.NotifyModemReset()

	if mgr.chipInfoCache != nil {
		t.Fatal("NotifyModemReset() did not clear chipInfoCache")
	}
	if len(mgr.discoveredEUICCs) != 0 {
		t.Fatalf("NotifyModemReset() discoveredEUICCs = %v, want empty", mgr.discoveredEUICCs)
	}
	if _, err := mgr.GetEsimOverview(); !errors.Is(err, reloadErr) {
		t.Fatalf("GetEsimOverview() error = %v, want %v after reset", err, reloadErr)
	}
}

func TestNotifyModemResetDelayedClearsCacheImmediatelyAndReloadsAfterDelay(t *testing.T) {
	loaded := make(chan struct{}, 1)
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		loaded <- struct{}{}
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "reloaded"}}, nil
	})
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "cached"}}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "cached"}
	mgr.discoveredEUICCs = []EUICCInfo{{AIDHex: "0102", EID: "eid-before-reset", Spec: EUICCSpecSGP22}}

	mgr.NotifyModemResetDelayed(80 * time.Millisecond)

	if mgr.cachedOverview() != nil {
		t.Fatal("overview cache should be cleared immediately")
	}
	if mgr.chipInfoCache != nil {
		t.Fatal("chipInfoCache should be cleared immediately")
	}
	if len(mgr.discoveredEUICCs) != 0 {
		t.Fatalf("discoveredEUICCs = %v, want cleared", mgr.discoveredEUICCs)
	}
	select {
	case <-loaded:
		t.Fatal("overview reloaded before delay elapsed")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-loaded:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("overview did not reload after delay")
	}
}

func TestNotifyModemResetDelayedSkipsReloadDuringSwitchSuppressionWindow(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "reloaded"}}, nil
	})
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "cached"}}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "cached"}
	mgr.discoveredEUICCs = []EUICCInfo{{AIDHex: "0102", EID: "eid-before-reset", Spec: EUICCSpecSGP22}}
	mgr.suppressOverviewReloadUntil = time.Now().Add(2 * time.Second)

	mgr.NotifyModemResetDelayed(80 * time.Millisecond)
	select {
	case <-time.After(200 * time.Millisecond):
	}

	if calls.Load() != 0 {
		t.Fatalf("overview reload calls = %d, want 0 during switch suppression window", calls.Load())
	}
	if got := mgr.cachedOverview(); got != nil {
		t.Fatalf("cachedOverview() = %#v, want cleared during reset suppression window", got)
	}
	if mgr.chipInfoCache != nil {
		t.Fatalf("chipInfoCache = %#v, want cleared during reset suppression window", mgr.chipInfoCache)
	}
	if len(mgr.discoveredEUICCs) != 0 {
		t.Fatalf("discoveredEUICCs = %v, want cleared during reset suppression window", mgr.discoveredEUICCs)
	}
}

func TestWarmOverviewAsyncLoadsInBackground(t *testing.T) {
	loaded := make(chan struct{}, 1)
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		loaded <- struct{}{}
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "warm"}}, nil
	})

	mgr.WarmOverviewAsync("startup")

	select {
	case <-loaded:
	case <-time.After(2 * time.Second):
		t.Fatal("WarmOverviewAsync() did not trigger background load")
	}

	if _, err := mgr.GetEsimOverview(); err != nil {
		t.Fatalf("GetEsimOverview() after warm load error = %v", err)
	}
}

func TestWarmOverviewAsyncWaitsForReloadSuppressionWindow(t *testing.T) {
	loaded := make(chan struct{}, 1)
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		loaded <- struct{}{}
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "warm"}}, nil
	})
	mgr.suppressOverviewReloadUntil = time.Now().Add(80 * time.Millisecond)

	mgr.WarmOverviewAsync("settle")

	select {
	case <-loaded:
		t.Fatal("WarmOverviewAsync() loaded before suppression window expired")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-loaded:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("WarmOverviewAsync() did not load after suppression window expired")
	}
}

func TestRefreshOverviewDuringDelayedBackgroundReloadUsesSingleLoad(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "fresh"}}, nil
	})
	mgr.invalidateOverviewCache("test")
	mgr.suppressOverviewReloadUntil = time.Now().Add(80 * time.Millisecond)
	mgr.WarmOverviewAsync("settle")

	if err := mgr.RefreshOverview(); err != nil {
		t.Fatalf("RefreshOverview() error = %v", err)
	}
	time.Sleep(120 * time.Millisecond)

	if calls.Load() != 1 {
		t.Fatalf("overview loader calls = %d, want one coalesced load", calls.Load())
	}
}

func TestInvalidateOverviewCacheDiscardsOlderReloadResult(t *testing.T) {
	firstRelease := make(chan struct{})
	started := make(chan struct{}, 1)
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		started <- struct{}{}
		<-firstRelease
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "stale"}}, nil
	})

	mgr.WarmOverviewAsync("startup")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first reload did not start")
	}

	mgr.invalidateOverviewCache("reset")
	close(firstRelease)

	select {
	case <-time.After(200 * time.Millisecond):
	}

	if got := mgr.cachedOverview(); got != nil {
		t.Fatalf("cachedOverview() = %#v, want nil after invalidation racing old reload", got)
	}
}

func TestPatchCachedActiveProfileClearsAllOtherProfilesOnDevice(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{
		ChipInfo: &EUICCChipInfo{SkuName: "chip"},
		Profiles: []EUICCProfiles{
			{
				EID:    "eid-a",
				AIDHex: "A000",
				Profiles: []ProfileItem{
					{ICCID: "iccid-a1", State: 1, StateText: "已启用"},
					{ICCID: "iccid-a2", State: 0, StateText: "已禁用"},
				},
			},
			{
				EID:    "eid-b",
				AIDHex: "B000",
				Profiles: []ProfileItem{
					{ICCID: "iccid-b1", State: 1, StateText: "已启用"},
					{ICCID: "iccid-b2", State: 0, StateText: "已禁用"},
				},
			},
		},
	}

	if ok := mgr.patchCachedActiveProfile("iccid-a2", "A000"); !ok {
		t.Fatal("patchCachedActiveProfile() = false, want true")
	}

	got := mgr.cachedOverview()
	if got == nil {
		t.Fatal("cachedOverview() = nil, want cached overview")
	}
	if got.ChipInfo == nil || got.ChipInfo.SkuName != "chip" {
		t.Fatalf("chipInfo = %#v, want unchanged chip info", got.ChipInfo)
	}
	if got.Profiles[0].Profiles[0].State != 0 || got.Profiles[0].Profiles[0].StateText != "已禁用" {
		t.Fatalf("first profile in target group = %#v, want disabled", got.Profiles[0].Profiles[0])
	}
	if got.Profiles[0].Profiles[1].State != 1 || got.Profiles[0].Profiles[1].StateText != "已启用" {
		t.Fatalf("target profile = %#v, want enabled", got.Profiles[0].Profiles[1])
	}
	if got.Profiles[1].Profiles[0].State != 0 || got.Profiles[1].Profiles[0].StateText != "已禁用" {
		t.Fatalf("other group's previously enabled profile = %#v, want disabled", got.Profiles[1].Profiles[0])
	}
	if got.Profiles[1].Profiles[1].State != 0 || got.Profiles[1].Profiles[1].StateText != "已禁用" {
		t.Fatalf("other group's inactive profile = %#v, want stay disabled", got.Profiles[1].Profiles[1])
	}
}

func TestPatchCachedActiveProfileClearsPreviouslyEnabledProfileAcrossAIDsWithinSameEID(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{
		ChipInfo: &EUICCChipInfo{SkuName: "chip"},
		Profiles: []EUICCProfiles{
			{
				EID:    "eid-shared",
				AIDHex: "SE1",
				Profiles: []ProfileItem{
					{ICCID: "iccid-se1-active", State: 1, StateText: "已启用"},
				},
			},
			{
				EID:    "eid-shared",
				AIDHex: "SE0",
				Profiles: []ProfileItem{
					{ICCID: "iccid-se0-target", State: 0, StateText: "已禁用"},
				},
			},
		},
	}

	if ok := mgr.patchCachedActiveProfile("iccid-se0-target", "SE0"); !ok {
		t.Fatal("patchCachedActiveProfile() = false, want true")
	}

	got := mgr.cachedOverview()
	if got == nil {
		t.Fatal("cachedOverview() = nil, want cached overview")
	}
	if got.Profiles[0].Profiles[0].State != 0 || got.Profiles[0].Profiles[0].StateText != "已禁用" {
		t.Fatalf("previously enabled profile = %#v, want disabled after cross-AID switch within same EID", got.Profiles[0].Profiles[0])
	}
	if got.Profiles[1].Profiles[0].State != 1 || got.Profiles[1].Profiles[0].StateText != "已启用" {
		t.Fatalf("target profile = %#v, want enabled", got.Profiles[1].Profiles[0])
	}
}

func TestActiveProfileNameReturnsFirstEnabledProfileByTraversalOrder(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{
		Profiles: []EUICCProfiles{
			{
				EID:    "eid-a",
				AIDHex: "A000",
				Profiles: []ProfileItem{
					{ICCID: "iccid-a1", Name: " First Enabled ", State: 1, StateText: "已启用"},
				},
			},
			{
				EID:    "eid-b",
				AIDHex: "B000",
				Profiles: []ProfileItem{
					{ICCID: "iccid-b1", Name: "Second Enabled", State: 1, StateText: "已启用"},
				},
			},
		},
	}

	got, err := mgr.ActiveProfileName()
	if err != nil {
		t.Fatalf("ActiveProfileName() error = %v", err)
	}
	if got != "First Enabled" {
		t.Fatalf("ActiveProfileName() = %q, want %q", got, "First Enabled")
	}
}

func TestActiveProfileNameDoesNotTriggerLoadWhenCacheMissing(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{
			Profiles: []EUICCProfiles{{
				EID:      "eid-a",
				AIDHex:   "A000",
				Profiles: []ProfileItem{{ICCID: "iccid-a1", Name: "Should Not Load", State: 1, StateText: "已启用"}},
			}},
		}, nil
	})

	got, err := mgr.ActiveProfileName()
	if err != nil {
		t.Fatalf("ActiveProfileName() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ActiveProfileName() = %q, want empty when cache missing", got)
	}
	if calls.Load() != 0 {
		t.Fatalf("overview loader calls = %d, want 0", calls.Load())
	}
}

func TestRefreshProfilesPreservesCachedChipInfo(t *testing.T) {
	var profileLoads atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{SkuName: "chip-before", Firmware: "1.0.0"},
			Profiles: []EUICCProfiles{{
				EID:      "eid-a",
				AIDHex:   "A000",
				Profiles: []ProfileItem{{ICCID: "iccid-a1", State: 1, StateText: "已启用"}},
			}},
		}, nil
	})
	mgr.profilesLoader = func() ([]EUICCProfiles, error) {
		profileLoads.Add(1)
		return []EUICCProfiles{{
			EID:      "eid-a",
			AIDHex:   "A000",
			Profiles: []ProfileItem{{ICCID: "iccid-a2", State: 1, StateText: "已启用"}},
		}}, nil
	}

	if _, err := mgr.GetEsimOverview(); err != nil {
		t.Fatalf("GetEsimOverview() error = %v", err)
	}
	if err := mgr.RefreshProfiles(); err != nil {
		t.Fatalf("RefreshProfiles() error = %v", err)
	}

	got := mgr.cachedOverview()
	if got == nil {
		t.Fatal("cachedOverview() = nil, want cached overview")
	}
	if got.ChipInfo == nil || got.ChipInfo.SkuName != "chip-before" || got.ChipInfo.Firmware != "1.0.0" {
		t.Fatalf("chipInfo = %#v, want original chip info preserved", got.ChipInfo)
	}
	if len(got.Profiles) != 1 || len(got.Profiles[0].Profiles) != 1 || got.Profiles[0].Profiles[0].ICCID != "iccid-a2" {
		t.Fatalf("profiles = %#v, want refreshed profiles only", got.Profiles)
	}
	if profileLoads.Load() != 1 {
		t.Fatalf("profiles loader calls = %d, want 1", profileLoads.Load())
	}
}

func TestRefreshOverviewReplacesChipInfoAndProfiles(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{
		ChipInfo: &EUICCChipInfo{SkuName: "before", Firmware: "1.0.0"},
		Profiles: []EUICCProfiles{{
			EID:      "eid-old",
			AIDHex:   "OLD",
			Profiles: []ProfileItem{{ICCID: "iccid-old", State: 1, StateText: "已启用"}},
		}},
	}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "before", Firmware: "1.0.0"}
	mgr.discoveredEUICCs = []EUICCInfo{{AIDHex: "0102", EID: "eid-before-refresh", Spec: EUICCSpecSGP22}}
	mgr.overviewLoader = func() (*EsimOverview, error) {
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{
				SkuName:  "after",
				Firmware: "2.0.0",
				EIDs:     []EUICCInfo{{EID: "eid-new", AIDHex: "NEW"}},
			},
			Profiles: []EUICCProfiles{{
				EID:      "eid-new",
				AIDHex:   "NEW",
				Profiles: []ProfileItem{{ICCID: "iccid-new", State: 1, StateText: "已启用"}},
			}},
		}, nil
	}

	if err := mgr.RefreshOverview(); err != nil {
		t.Fatalf("RefreshOverview() error = %v", err)
	}

	got := mgr.cachedOverview()
	if got == nil {
		t.Fatal("cachedOverview() = nil, want refreshed overview")
	}
	if got.ChipInfo == nil || got.ChipInfo.SkuName != "after" || got.ChipInfo.Firmware != "2.0.0" {
		t.Fatalf("chipInfo = %#v, want refreshed chip info", got.ChipInfo)
	}
	if len(got.Profiles) != 1 || got.Profiles[0].AIDHex != "NEW" || got.Profiles[0].Profiles[0].ICCID != "iccid-new" {
		t.Fatalf("profiles = %#v, want refreshed profiles", got.Profiles)
	}
	if mgr.chipInfoCache == nil || mgr.chipInfoCache.SkuName != "after" || mgr.chipInfoCache.Firmware != "2.0.0" {
		t.Fatalf("chipInfoCache = %#v, want refreshed chipInfoCache", mgr.chipInfoCache)
	}
	if len(mgr.discoveredEUICCs) != 0 {
		t.Fatalf("discoveredEUICCs = %v, want cleared by manual refresh", mgr.discoveredEUICCs)
	}
}

func TestRefreshOverviewPreventsInFlightStaleLoadFromOverwritingFreshSnapshot(t *testing.T) {
	staleRelease := make(chan struct{})
	staleStarted := make(chan struct{}, 1)
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		if calls.Add(1) == 1 {
			staleStarted <- struct{}{}
			<-staleRelease
			return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "stale"}}, nil
		}
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "fresh"}}, nil
	})

	loadDone := make(chan struct{})
	go func() {
		defer close(loadDone)
		if _, err := mgr.loadOverview(); err != nil {
			t.Errorf("loadOverview() error = %v", err)
		}
	}()

	select {
	case <-staleStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("stale loadOverview() did not start")
	}

	if err := mgr.RefreshOverview(); err != nil {
		t.Fatalf("RefreshOverview() error = %v", err)
	}

	close(staleRelease)
	select {
	case <-loadDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stale loadOverview() did not finish")
	}

	got := mgr.cachedOverview()
	if got == nil || got.ChipInfo == nil || got.ChipInfo.SkuName != "fresh" {
		t.Fatalf("cachedOverview() = %#v, want fresh snapshot preserved", got)
	}
}

func TestRefreshOverviewClearsChipInfoCacheWhenRefreshHasNoChipInfo(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "before"}}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "before"}
	mgr.overviewLoader = func() (*EsimOverview, error) {
		return &EsimOverview{
			ChipInfo: nil,
			Profiles: []EUICCProfiles{{
				EID:      "eid-new",
				AIDHex:   "NEW",
				Profiles: []ProfileItem{{ICCID: "iccid-new"}},
			}},
		}, nil
	}

	if err := mgr.RefreshOverview(); err != nil {
		t.Fatalf("RefreshOverview() error = %v", err)
	}

	got := mgr.cachedOverview()
	if got == nil {
		t.Fatal("cachedOverview() = nil, want refreshed overview")
	}
	if got.ChipInfo != nil {
		t.Fatalf("cachedOverview().ChipInfo = %#v, want nil", got.ChipInfo)
	}
	if mgr.chipInfoCache != nil {
		t.Fatalf("chipInfoCache = %#v, want nil", mgr.chipInfoCache)
	}
}

func TestRefreshOverviewClearsEmptyChipInfoCacheBeforeReload(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{
		EIDs: []EUICCInfo{{EID: "eid-old", AIDHex: "OLD"}},
	}}
	mgr.chipInfoCache = &EUICCChipInfo{
		EIDs: []EUICCInfo{{EID: "eid-old", AIDHex: "OLD"}},
	}
	var sawClearedCache bool
	mgr.overviewLoader = func() (*EsimOverview, error) {
		mgr.cacheMu.RLock()
		cached := mgr.chipInfoCache
		mgr.cacheMu.RUnlock()
		sawClearedCache = cached == nil
		return &EsimOverview{
			ChipInfo: &EUICCChipInfo{EIDs: []EUICCInfo{{EID: "eid-new", AIDHex: "NEW"}}},
		}, nil
	}

	if err := mgr.RefreshOverview(); err != nil {
		t.Fatalf("RefreshOverview() error = %v", err)
	}

	if !sawClearedCache {
		t.Fatal("RefreshOverview() did not clear empty chipInfoCache before reload")
	}
}

func TestNotifyUIMIndicationSkipsReloadDuringSwitchSuppressionWindow(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "reloaded"}}, nil
	})
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "cached"}}
	mgr.suppressOverviewReloadUntil = time.Now().Add(2 * time.Second)

	mgr.NotifyUIMIndication("slot_status")
	select {
	case <-time.After(200 * time.Millisecond):
	}

	if calls.Load() != 0 {
		t.Fatalf("overview reload calls = %d, want 0 during switch suppression window", calls.Load())
	}
	if got := mgr.cachedOverview(); got == nil || got.ChipInfo == nil || got.ChipInfo.SkuName != "cached" {
		t.Fatalf("cachedOverview() = %#v, want cached overview unchanged", got)
	}
}

func TestNotifyModemResetSkipsReloadDuringSwitchSuppressionWindow(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(func() (*EsimOverview, error) {
		calls.Add(1)
		return &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "reloaded"}}, nil
	})
	mgr.overviewCache = &EsimOverview{ChipInfo: &EUICCChipInfo{SkuName: "cached"}}
	mgr.chipInfoCache = &EUICCChipInfo{SkuName: "cached"}
	mgr.discoveredEUICCs = []EUICCInfo{{AIDHex: "0102", EID: "eid-before-reset", Spec: EUICCSpecSGP22}}
	mgr.suppressOverviewReloadUntil = time.Now().Add(2 * time.Second)

	mgr.NotifyModemReset()
	select {
	case <-time.After(200 * time.Millisecond):
	}

	if calls.Load() != 0 {
		t.Fatalf("overview reload calls = %d, want 0 during switch suppression window", calls.Load())
	}
	if got := mgr.cachedOverview(); got != nil {
		t.Fatalf("cachedOverview() = %#v, want cleared during reset suppression window", got)
	}
	if mgr.chipInfoCache != nil {
		t.Fatalf("chipInfoCache = %#v, want cleared during reset suppression window", mgr.chipInfoCache)
	}
	if len(mgr.discoveredEUICCs) != 0 {
		t.Fatalf("discoveredEUICCs = %v, want cleared during reset suppression window", mgr.discoveredEUICCs)
	}
}

func TestDeleteProfileResultPreservesWarningDetails(t *testing.T) {
	result := DeleteProfileResult{
		Warning:     "Profile 已删除，但删除通知发送未完全确认",
		WarningCode: "delete_notification_not_observed",
		SpaceDelta: &SpaceDelta{
			Direction: SpaceDeltaDirectionReleased,
			Bytes:     4096,
		},
	}

	if result.Warning != "Profile 已删除，但删除通知发送未完全确认" {
		t.Fatalf("Warning=%q want delete warning", result.Warning)
	}
	if result.WarningCode != "delete_notification_not_observed" {
		t.Fatalf("WarningCode=%q want delete_notification_not_observed", result.WarningCode)
	}
	if result.SpaceDelta == nil || result.SpaceDelta.Direction != SpaceDeltaDirectionReleased || result.SpaceDelta.Bytes != 4096 {
		t.Fatalf("SpaceDelta=%#v want released/4096", result.SpaceDelta)
	}
}

func TestBuildSpaceDeltaForDeleteReturnsReleasedBytes(t *testing.T) {
	delta := buildSpaceDeltaForOperation(spaceDeltaOperationDelete, 1024, 2048)
	if delta == nil {
		t.Fatal("buildSpaceDeltaForOperation() = nil, want released delta")
	}
	if delta.Direction != SpaceDeltaDirectionReleased || delta.Bytes != 1024 {
		t.Fatalf("delta=%#v want released/1024", delta)
	}
}

func TestBuildSpaceDeltaForDownloadReturnsConsumedBytes(t *testing.T) {
	delta := buildSpaceDeltaForOperation(spaceDeltaOperationDownload, 4096, 1024)
	if delta == nil {
		t.Fatal("buildSpaceDeltaForOperation() = nil, want consumed delta")
	}
	if delta.Direction != SpaceDeltaDirectionConsumed || delta.Bytes != 3072 {
		t.Fatalf("delta=%#v want consumed/3072", delta)
	}
}

func TestBuildSpaceDeltaForOperationOmitsInvalidSnapshots(t *testing.T) {
	testCases := []struct {
		name   string
		op     spaceDeltaOperation
		before int32
		after  int32
	}{
		{name: "before zero", op: spaceDeltaOperationDelete, before: 0, after: 100},
		{name: "after zero", op: spaceDeltaOperationDelete, before: 100, after: 0},
		{name: "delete non-growth", op: spaceDeltaOperationDelete, before: 200, after: 200},
		{name: "download non-drop", op: spaceDeltaOperationDownload, before: 200, after: 300},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if delta := buildSpaceDeltaForOperation(tc.op, tc.before, tc.after); delta != nil {
				t.Fatalf("buildSpaceDeltaForOperation() = %#v, want nil", delta)
			}
		})
	}
}

func TestDeleteNotificationWarningCodesUseDeleteScopedNames(t *testing.T) {
	retrieveErr := errors.New("retrieve failed")
	if got := deleteNotificationResult(true, retrieveErr, nil); got.WarningCode != "delete_notification_retrieve_failed" {
		t.Fatalf("retrieve warning=%#v want delete_notification_retrieve_failed", got)
	}

	handleErr := errors.New("handle failed")
	if got := deleteNotificationResult(true, nil, handleErr); got.WarningCode != "delete_notification_handle_failed" {
		t.Fatalf("handle warning=%#v want delete_notification_handle_failed", got)
	}
}

func TestFindDeleteNotificationWaitsForMatchingNotification(t *testing.T) {
	wantICCID, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}

	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(nil)
	notification, ok := mgr.findDeleteNotificationWithWait(func() ([]*sgp22.NotificationMetadata, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return []*sgp22.NotificationMetadata{{
			SequenceNumber:             7,
			ProfileManagementOperation: sgp22.NotificationEventDelete,
			ICCID:                      wantICCID,
		}}, nil
	}, 6, wantICCID, 3, time.Millisecond)
	if !ok {
		t.Fatal("findDeleteNotificationWithWait() ok=false, want true")
	}
	if notification == nil || notification.SequenceNumber != 7 {
		t.Fatalf("notification=%#v want matching sequence=7", notification)
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls=%d want 2", calls.Load())
	}
}

func TestFindNotificationWithWaitReturnsFirstNewNotification(t *testing.T) {
	var calls atomic.Int32
	mgr := newTestManagerWithOverviewLoader(nil)
	notification, ok := mgr.findNotificationWithWait(func() ([]*sgp22.NotificationMetadata, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return []*sgp22.NotificationMetadata{{SequenceNumber: 11}}, nil
	}, 10, 3, time.Millisecond)
	if !ok {
		t.Fatal("findNotificationWithWait() ok=false, want true")
	}
	if notification == nil || notification.SequenceNumber != 11 {
		t.Fatalf("notification=%#v want matching sequence=11", notification)
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls=%d want 2", calls.Load())
	}
}

func TestDeleteProfileReturnsZeroWarningResultForInvalidICCID(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)

	result, err := mgr.DeleteProfile("bad-iccid", "")
	if err == nil {
		t.Fatal("DeleteProfile() error=nil, want invalid ICCID error")
	}
	if result.Warning != "" || result.WarningCode != "" {
		t.Fatalf("result=%#v want zero warning result on hard error", result)
	}
}

func TestDisableProfileRejectsInvalidICCIDBeforeOpeningChannel(t *testing.T) {
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		t.Fatalf("channel factory called for aid=%X", aid)
		return nil, nil
	}, nil, nil, nil)

	err := mgr.DisableProfile(context.Background(), "bad-iccid", "")
	if err == nil {
		t.Fatal("DisableProfile() error=nil, want invalid ICCID error")
	}
}

func TestDownloadProfileReturnsZeroWarningResultForInvalidAIDHex(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)

	result, err := mgr.DownloadProfile(context.Background(), "zz", "example.com", "", "", "", nil)
	if err == nil {
		t.Fatal("DownloadProfile() error=nil, want invalid AID error")
	}
	if result.Warning != "" || result.WarningCode != "" {
		t.Fatalf("result=%#v want zero warning result on hard error", result)
	}
}

func TestResolveDownloadIMEIUsesExplicitCustomIMEIBeforeProviders(t *testing.T) {
	mgr := newManagerWithChannelFactory("reader-slot-001", func(aid []byte) (*lpa.Client, error) {
		t.Fatalf("channel factory called for aid=%X", aid)
		return nil, nil
	}, nil, nil, nil)
	mgr.imeiProvider = func(ctx context.Context) (string, error) {
		t.Fatal("IMEIProvider called despite explicit IMEI")
		return "", nil
	}

	imei, err := mgr.resolveDownloadIMEI(context.Background(), "350225641234561")
	if err != nil {
		t.Fatalf("resolveDownloadIMEI() error=%v", err)
	}
	if imei != "350225641234561" {
		t.Fatalf("imei=%q want explicit custom IMEI", imei)
	}
}

func TestResolveDownloadIMEIRejectsInvalidExplicitCustomIMEI(t *testing.T) {
	mgr := newManagerWithChannelFactory("reader-slot-001", func(aid []byte) (*lpa.Client, error) {
		t.Fatalf("channel factory called for aid=%X", aid)
		return nil, nil
	}, nil, nil, nil)

	_, err := mgr.resolveDownloadIMEI(context.Background(), "350225641234560")
	if err == nil {
		t.Fatal("resolveDownloadIMEI() error=nil, want invalid IMEI error")
	}
	if !strings.Contains(err.Error(), "无效的 IMEI") {
		t.Fatalf("error=%q want invalid IMEI message", err)
	}
}

func TestResolveDownloadIMEIDoesNotGenerateSyntheticIMEIForCustomTransport(t *testing.T) {
	mgr := newManagerWithChannelFactory("reader-slot-001", func(aid []byte) (*lpa.Client, error) {
		t.Fatalf("channel factory called for aid=%X", aid)
		return nil, nil
	}, nil, nil, nil)

	_, err := mgr.resolveDownloadIMEI(context.Background(), "")
	if err == nil {
		t.Fatal("resolveDownloadIMEI() error=nil, want missing IMEI error")
	}
	if !strings.Contains(err.Error(), "无法获取设备 IMEI") {
		t.Fatalf("error=%q want missing IMEI message", err)
	}
}

func TestClassifyDownloadErrorIdentifiesInsufficientMemory(t *testing.T) {
	err := sgp22.LoadBoundProfilePackageError{BPPCommandID: 5, ErrorReason: 10}

	info := ClassifyDownloadError(err)
	if info.Code != DownloadErrorEUICCInsufficientMemory {
		t.Fatalf("Code=%q want %q", info.Code, DownloadErrorEUICCInsufficientMemory)
	}
	if info.BPPCommandID != 5 || info.BPPErrorReason != 10 {
		t.Fatalf("BPP fields=(%d,%d) want (5,10)", info.BPPCommandID, info.BPPErrorReason)
	}
	if !strings.Contains(info.Message, "空间不足") {
		t.Fatalf("Message=%q want mention space shortage", info.Message)
	}
}

func TestClassifyDownloadErrorDoesNotTreatNonLoadProfileElementsReason10AsInsufficientMemory(t *testing.T) {
	err := sgp22.LoadBoundProfilePackageError{BPPCommandID: 2, ErrorReason: 10}

	info := ClassifyDownloadError(err)
	if info.Code != DownloadErrorEUICCProfileInstallFailed {
		t.Fatalf("Code=%q want %q", info.Code, DownloadErrorEUICCProfileInstallFailed)
	}
	if strings.Contains(info.Message, "空间不足") {
		t.Fatalf("Message=%q should not mention space shortage for command id 2", info.Message)
	}
}

func TestClassifyDownloadErrorSeesWrappedBPPError(t *testing.T) {
	baseErr := &sgp22.LoadBoundProfilePackageError{BPPCommandID: 5, ErrorReason: 9}
	err := fmt.Errorf("下载 profile 失败: %w (cancel session error: remote failed)", baseErr)

	info := ClassifyDownloadError(err)
	if info.Code != DownloadErrorEUICCIccidAlreadyExists {
		t.Fatalf("Code=%q want %q", info.Code, DownloadErrorEUICCIccidAlreadyExists)
	}
	if info.BPPCommandID != 5 || info.BPPErrorReason != 9 {
		t.Fatalf("BPP fields=(%d,%d) want (5,9)", info.BPPCommandID, info.BPPErrorReason)
	}
}

func TestClassifyDownloadErrorUsesGenericProfileInstallCodeForUnknownBPPError(t *testing.T) {
	err := sgp22.LoadBoundProfilePackageError{BPPCommandID: 5, ErrorReason: 99}

	info := ClassifyDownloadError(err)
	if info.Code != DownloadErrorEUICCProfileInstallFailed {
		t.Fatalf("Code=%q want %q", info.Code, DownloadErrorEUICCProfileInstallFailed)
	}
}

func TestClassifyDownloadErrorUsesGenericDownloadCodeForNonBPPError(t *testing.T) {
	info := ClassifyDownloadError(errors.New("network down"))
	if info.Code != DownloadErrorGeneric {
		t.Fatalf("Code=%q want %q", info.Code, DownloadErrorGeneric)
	}
	if info.BPPCommandID != 0 || info.BPPErrorReason != 0 {
		t.Fatalf("BPP fields=(%d,%d) want zero values", info.BPPCommandID, info.BPPErrorReason)
	}
}

func TestDownloadProfileErrorPreservesOriginalErrorForErrorsAs(t *testing.T) {
	baseErr := &sgp22.LoadBoundProfilePackageError{BPPCommandID: 5, ErrorReason: 10}
	err := NewDownloadProfileError(baseErr)

	var bppErr *sgp22.LoadBoundProfilePackageError
	if !errors.As(err, &bppErr) {
		t.Fatal("errors.As() did not find LoadBoundProfilePackageError")
	}
	if err.Code != DownloadErrorEUICCInsufficientMemory {
		t.Fatalf("Code=%q want %q", err.Code, DownloadErrorEUICCInsufficientMemory)
	}
}

func TestDownloadNotificationResultWarningCodes(t *testing.T) {
	if got := downloadNotificationResult(false, nil, nil); got.WarningCode != "download_notification_not_observed" {
		t.Fatalf("no-notification warning=%#v want download_notification_not_observed", got)
	}

	retrieveErr := errors.New("retrieve failed")
	if got := downloadNotificationResult(true, retrieveErr, nil); got.WarningCode != "download_notification_retrieve_failed" {
		t.Fatalf("retrieve warning=%#v want download_notification_retrieve_failed", got)
	}

	handleErr := errors.New("handle failed")
	if got := downloadNotificationResult(true, nil, handleErr); got.WarningCode != "download_notification_handle_failed" {
		t.Fatalf("handle warning=%#v want download_notification_handle_failed", got)
	}

	if got := downloadNotificationResult(true, nil, nil); got.Warning != "" || got.WarningCode != "" {
		t.Fatalf("success warning=%#v want zero warning result", got)
	}
}

func TestDeleteNotificationResultWarningCodes(t *testing.T) {
	if got := deleteNotificationResult(false, nil, nil); got.WarningCode != "delete_notification_not_observed" {
		t.Fatalf("not-observed warning=%#v want delete_notification_not_observed", got)
	}

	retrieveErr := errors.New("retrieve failed")
	if got := deleteNotificationResult(true, retrieveErr, nil); got.WarningCode != "delete_notification_retrieve_failed" {
		t.Fatalf("retrieve warning=%#v want delete_notification_retrieve_failed", got)
	}

	handleErr := errors.New("handle failed")
	if got := deleteNotificationResult(true, nil, handleErr); got.WarningCode != "delete_notification_handle_failed" {
		t.Fatalf("handle warning=%#v want delete_notification_handle_failed", got)
	}

	if got := deleteNotificationResult(true, nil, nil); got.Warning != "" || got.WarningCode != "" {
		t.Fatalf("success warning=%#v want zero warning result", got)
	}
}

func TestResolveDeleteNotificationResultUsesDelayedObservationAndFailureReason(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	mgr := newTestManagerWithOverviewLoader(nil)
	metadata := &sgp22.NotificationMetadata{SequenceNumber: 7, ICCID: iccid}

	var calls atomic.Int32
	got := mgr.resolveDeleteNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 6, iccid, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		if seq != 7 {
			t.Fatalf("seq=%d want 7", seq)
		}
		return nil
	}, func(seq sgp22.SequenceNumber) error {
		if seq != 7 {
			t.Fatalf("seq=%d want 7", seq)
		}
		return nil
	})
	if got.Warning != "" || got.WarningCode != "" {
		t.Fatalf("got=%#v want clean result after delayed observation", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls=%d want 2", calls.Load())
	}

	got = mgr.resolveDeleteNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 6, iccid, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		return errors.New("retrieve failed")
	}, func(seq sgp22.SequenceNumber) error {
		return nil
	})
	if got.WarningCode != "delete_notification_retrieve_failed" {
		t.Fatalf("got=%#v want delete_notification_retrieve_failed", got)
	}

	got = mgr.resolveDeleteNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 6, iccid, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		return nil
	}, func(seq sgp22.SequenceNumber) error {
		return errors.New("handle failed")
	})
	if got.WarningCode != "delete_notification_handle_failed" {
		t.Fatalf("got=%#v want delete_notification_handle_failed", got)
	}
}

func TestResolveDownloadNotificationResultUsesDelayedObservationAndFailureReason(t *testing.T) {
	mgr := newTestManagerWithOverviewLoader(nil)
	metadata := &sgp22.NotificationMetadata{SequenceNumber: 11}

	var calls atomic.Int32
	got := mgr.resolveDownloadNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		if calls.Add(1) == 1 {
			return nil, nil
		}
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 10, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		if seq != 11 {
			t.Fatalf("seq=%d want 11", seq)
		}
		return nil
	}, func(seq sgp22.SequenceNumber) error {
		if seq != 11 {
			t.Fatalf("seq=%d want 11", seq)
		}
		return nil
	})
	if got.Warning != "" || got.WarningCode != "" {
		t.Fatalf("got=%#v want clean result after delayed observation", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("list calls=%d want 2", calls.Load())
	}

	got = mgr.resolveDownloadNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 10, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		return errors.New("retrieve failed")
	}, func(seq sgp22.SequenceNumber) error {
		return nil
	})
	if got.WarningCode != "download_notification_retrieve_failed" {
		t.Fatalf("got=%#v want download_notification_retrieve_failed", got)
	}

	got = mgr.resolveDownloadNotificationResult(func() ([]*sgp22.NotificationMetadata, error) {
		return []*sgp22.NotificationMetadata{metadata}, nil
	}, 10, time.Millisecond, func(seq sgp22.SequenceNumber) error {
		return nil
	}, func(seq sgp22.SequenceNumber) error {
		return errors.New("handle failed")
	})
	if got.WarningCode != "download_notification_handle_failed" {
		t.Fatalf("got=%#v want download_notification_handle_failed", got)
	}
}

func TestDownloadNotificationBaselineUsesGreaterOfPreOpAndResultSequence(t *testing.T) {
	preOp := []*sgp22.NotificationMetadata{{SequenceNumber: 5}, {SequenceNumber: 8}}
	resultNotification := &sgp22.NotificationMetadata{SequenceNumber: 3}

	got := downloadNotificationBaseline(preOp, resultNotification)
	if got != 8 {
		t.Fatalf("baseline=%d want 8", got)
	}

	got = downloadNotificationBaseline(preOp, &sgp22.NotificationMetadata{SequenceNumber: 12})
	if got != 11 {
		t.Fatalf("baseline=%d want 11", got)
	}
}

func TestSafeListNotificationConvertsMalformedResponsePanicToError(t *testing.T) {
	client := &lpa.Client{APDU: malformedListNotificationTransmitter{}}

	notifications, err := safeListNotification(client, sgp22.NotificationEventInstall)
	if err == nil {
		t.Fatal("safeListNotification() error=nil, want parser panic converted to error")
	}
	if notifications != nil {
		t.Fatalf("notifications=%v want nil on malformed response", notifications)
	}
	if !strings.Contains(err.Error(), "解析通知列表响应失败") {
		t.Fatalf("error=%q want parse failure context", err)
	}
}

func TestRecoverDownloadInstallFinalizeErrorSendsInstallNotification(t *testing.T) {
	const aidHex = "A0000005591010FFFFFFFF8900000100"
	metadata := &sgp22.NotificationMetadata{
		SequenceNumber:             12,
		ProfileManagementOperation: sgp22.NotificationEventInstall,
		Address:                    "install.example.com",
	}
	retrieve := map[sgp22.SequenceNumber][]*sgp22.PendingNotification{
		12: {{
			PendingNotification: bertlv.NewChildren(bertlv.Universal.Constructed(16),
				bertlv.NewChildren(bertlv.ContextSpecific.Constructed(47),
					bertlv.NewValue(bertlv.Universal.Primitive(12), []byte("install.example.com")),
				),
			),
			Notification: metadata,
		}},
	}
	var factoryCalls atomic.Int32
	var closeCalls atomic.Int32
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		factoryCalls.Add(1)
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != aidHex {
			t.Fatalf("aid=%s want %s", got, aidHex)
		}
		client, rt := newTestNotificationClient([]*sgp22.NotificationMetadata{metadata}, retrieve, nil, nil)
		t.Cleanup(func() {
			if len(rt.handledHosts) != 1 || rt.handledHosts[0] != "install.example.com" {
				t.Fatalf("handledHosts=%v want one install notification", rt.handledHosts)
			}
		})
		return client, nil
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error {
		closeCalls.Add(1)
		return nil
	}

	result, ok := mgr.recoverDownloadInstallFinalizeError(
		context.Background(),
		mustDecodeHex(t, aidHex),
		[]*sgp22.NotificationMetadata{{SequenceNumber: 11}},
		errors.New("APDU 透传失败: 设备返回错误: ERROR (cancel session error: Execution Error)"),
	)
	if !ok {
		t.Fatal("recoverDownloadInstallFinalizeError() ok=false, want true")
	}
	if result.WarningCode != "download_installed_finalize_error_recovered" {
		t.Fatalf("WarningCode=%q want download_installed_finalize_error_recovered", result.WarningCode)
	}
	if factoryCalls.Load() != 1 {
		t.Fatalf("factory calls=%d want 1", factoryCalls.Load())
	}
	if closeCalls.Load() != 1 {
		t.Fatalf("close calls=%d want 1", closeCalls.Load())
	}
}

type fakeNotificationTransmitter struct {
	list        []*sgp22.NotificationMetadata
	retrieve    map[sgp22.SequenceNumber][]*sgp22.PendingNotification
	retrieveErr map[sgp22.SequenceNumber]error
	removeErr   map[sgp22.SequenceNumber]error
	retrieved   []sgp22.SequenceNumber
	removed     []sgp22.SequenceNumber
}

func (f *fakeNotificationTransmitter) Transmit(request bertlv.Marshaler, response bertlv.Unmarshaler) error {
	switch req := request.(type) {
	case *sgp22.ListNotificationRequest:
		resp, ok := response.(*sgp22.ListNotificationResponse)
		if !ok {
			return errors.New("unexpected list response type")
		}
		resp.NotificationList = f.list
		return nil
	case *sgp22.RetrieveNotificationsListRequest:
		resp, ok := response.(*sgp22.RetrieveNotificationsListResponse)
		if !ok {
			return errors.New("unexpected retrieve response type")
		}
		if req.SearchCriteria == nil {
			return errors.New("missing search criteria")
		}
		var seq sgp22.SequenceNumber
		if err := req.SearchCriteria.UnmarshalValue(primitive.UnmarshalInt(&seq)); err != nil {
			return err
		}
		f.retrieved = append(f.retrieved, seq)
		if err := f.retrieveErr[seq]; err != nil {
			return err
		}
		resp.NotificationList = f.retrieve[seq]
		return nil
	case *sgp22.NotificationSentRequest:
		resp, ok := response.(*sgp22.NotificationSentResponse)
		if !ok {
			return errors.New("unexpected notification sent response type")
		}
		if err := f.removeErr[req.SequenceNumber]; err != nil {
			return err
		}
		resp.DeleteNotificationStatus = 0
		f.removed = append(f.removed, req.SequenceNumber)
		kept := make([]*sgp22.NotificationMetadata, 0, len(f.list))
		for _, notification := range f.list {
			if notification != nil && notification.SequenceNumber == req.SequenceNumber {
				continue
			}
			kept = append(kept, notification)
		}
		f.list = kept
		return nil
	default:
		return errors.New("unexpected request type")
	}
}

func (f *fakeNotificationTransmitter) TransmitRaw(command []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

type malformedListNotificationTransmitter struct{}

func (malformedListNotificationTransmitter) Transmit(request bertlv.Marshaler, response bertlv.Unmarshaler) error {
	var tlv bertlv.TLV
	if err := tlv.UnmarshalBinary([]byte{0xBF, 0x28, 0x03, 0x81, 0x01, 0x7F}); err != nil {
		return err
	}
	return response.UnmarshalBERTLV(&tlv)
}

func (malformedListNotificationTransmitter) TransmitRaw(command []byte) ([]byte, error) {
	return nil, errors.New("not implemented")
}

type fakeNotificationRoundTripper struct {
	handledHosts    []string
	handleErrByHost map[string]error
}

func (f *fakeNotificationRoundTripper) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	f.handledHosts = append(f.handledHosts, req.URL.Host)
	if err := f.handleErrByHost[req.URL.Host]; err != nil {
		return nil, err
	}
	return &stdhttp.Response{
		StatusCode: 200,
		Header:     make(stdhttp.Header),
		Body:       io.NopCloser(strings.NewReader(`{"header":{}}`)),
		Request:    req,
	}, nil
}

func newTestNotificationClient(list []*sgp22.NotificationMetadata, retrieve map[sgp22.SequenceNumber][]*sgp22.PendingNotification, retrieveErr map[sgp22.SequenceNumber]error, handleErrByHost map[string]error) (*lpa.Client, *fakeNotificationRoundTripper) {
	client, roundTripper, _ := newTestNotificationClientWithTransmitter(list, retrieve, retrieveErr, nil, handleErrByHost)
	return client, roundTripper
}

func newTestNotificationClientWithTransmitter(list []*sgp22.NotificationMetadata, retrieve map[sgp22.SequenceNumber][]*sgp22.PendingNotification, retrieveErr map[sgp22.SequenceNumber]error, removeErr map[sgp22.SequenceNumber]error, handleErrByHost map[string]error) (*lpa.Client, *fakeNotificationRoundTripper, *fakeNotificationTransmitter) {
	roundTripper := &fakeNotificationRoundTripper{handleErrByHost: handleErrByHost}
	transmitter := &fakeNotificationTransmitter{list: list, retrieve: retrieve, retrieveErr: retrieveErr, removeErr: removeErr}
	return &lpa.Client{
		APDU: transmitter,
		HTTP: &euicchttp.Client{Client: &stdhttp.Client{Transport: roundTripper}, AdminProtocolVersion: "2.5.0"},
	}, roundTripper, transmitter
}

func testPendingNotification(seq sgp22.SequenceNumber, event sgp22.NotificationEvent, iccid sgp22.ICCID, address string) *sgp22.PendingNotification {
	return &sgp22.PendingNotification{
		PendingNotification: bertlv.NewValue(bertlv.ContextSpecific.Primitive(0), []byte{byte(seq)}),
		Notification:        &sgp22.NotificationMetadata{SequenceNumber: seq, ProfileManagementOperation: event, ICCID: iccid, Address: address},
	}
}

type lifecycleSmartCardChannel struct {
	response      []byte
	closeErr      error
	disconnectErr error
	channel       byte

	connectCalls    atomic.Int32
	openCalls       atomic.Int32
	transmitCalls   atomic.Int32
	closeCalls      atomic.Int32
	disconnectCalls atomic.Int32
	closedChannel   atomic.Int32
}

func (c *lifecycleSmartCardChannel) Connect() error {
	c.connectCalls.Add(1)
	return nil
}

func (c *lifecycleSmartCardChannel) Disconnect() error {
	c.disconnectCalls.Add(1)
	return c.disconnectErr
}

func (c *lifecycleSmartCardChannel) OpenLogicalChannel(aid []byte) (byte, error) {
	c.openCalls.Add(1)
	if c.channel == 0 {
		c.channel = 1
	}
	return c.channel, nil
}

func (c *lifecycleSmartCardChannel) Transmit(command []byte) ([]byte, error) {
	c.transmitCalls.Add(1)
	resp := append([]byte{}, c.response...)
	resp = append(resp, 0x90, 0x00)
	return resp, nil
}

func (c *lifecycleSmartCardChannel) CloseLogicalChannel(channel byte) error {
	c.closeCalls.Add(1)
	c.closedChannel.Store(int32(channel))
	return c.closeErr
}

var _ driver.SmartCardChannel = (*lifecycleSmartCardChannel)(nil)

func newLifecycleNotificationClient(t *testing.T, ch *lifecycleSmartCardChannel, handleErrByHost map[string]error) (*lpa.Client, *fakeNotificationRoundTripper) {
	t.Helper()
	client, err := lpa.New(&lpa.Options{Channel: ch, AID: lpa.GSMAISDRApplicationAID, MSS: 120})
	if err != nil {
		t.Fatalf("lpa.New() error=%v", err)
	}
	roundTripper := &fakeNotificationRoundTripper{handleErrByHost: handleErrByHost}
	client.HTTP = &euicchttp.Client{Client: &stdhttp.Client{Transport: roundTripper}, AdminProtocolVersion: "2.5.0"}
	return client, roundTripper
}

func lifecycleRetrieveNotificationResponse(t *testing.T) []byte {
	t.Helper()
	seq := sgp22.SequenceNumber(11)
	seqTLV, err := bertlv.MarshalValue(bertlv.ContextSpecific.Primitive(0), seq)
	if err != nil {
		t.Fatalf("marshal sequence: %v", err)
	}
	event := sgp22.NotificationEventInstall
	eventTLV, err := bertlv.MarshalValue(bertlv.ContextSpecific.Primitive(1), &event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	metadata := bertlv.NewChildren(bertlv.ContextSpecific.Constructed(47),
		bertlv.NewValue(bertlv.Universal.Primitive(12), []byte("install.example.com")),
		seqTLV,
		eventTLV,
	)
	pendingNotification := bertlv.NewChildren(bertlv.Universal.Constructed(16), metadata)
	response := bertlv.NewChildren(bertlv.ContextSpecific.Constructed(43),
		bertlv.NewChildren(bertlv.ContextSpecific.Constructed(0), pendingNotification),
	)
	return response.Bytes()
}

func emptyRetrieveNotificationResponse() []byte {
	return bertlv.NewChildren(bertlv.ContextSpecific.Constructed(43)).Bytes()
}

func TestListNotificationsMapsCurrentNotificationItems(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		if got := strings.ToUpper(hex.EncodeToString(aid)); got != "A0000005591010FFFFFFFF8900000100" {
			t.Fatalf("aid=%s want GSMA default AID", got)
		}
		client, _ := newTestNotificationClient([]*sgp22.NotificationMetadata{
			{SequenceNumber: 9, ProfileManagementOperation: sgp22.NotificationEventDelete, ICCID: iccid, Address: "delete.example.com"},
			{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"},
		}, nil, nil, nil)
		return client, nil
	}, nil, nil, nil)
	var waitCalled atomic.Int32
	mgr.apduArbiter = fakeAPDUIdleWaiter{wait: func(ctx context.Context) error {
		waitCalled.Add(1)
		return nil
	}}
	mgr.readQueueWaitTimeout = 200 * time.Millisecond

	items, err := mgr.ListNotifications("A0000005591010FFFFFFFF8900000100")
	if err != nil {
		t.Fatalf("ListNotifications() error=%v", err)
	}
	if waitCalled.Load() != 1 {
		t.Fatalf("WaitIdle calls=%d want 1", waitCalled.Load())
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d want 2", len(items))
	}
	if items[0].SequenceNumber != 11 || items[0].Event != "install" || items[0].ICCID != "8986001234567890123" || items[0].Address != "install.example.com" || !items[0].CanRetry {
		t.Fatalf("items[0]=%#v want mapped install notification", items[0])
	}
	if items[1].SequenceNumber != 9 || items[1].Event != "delete" {
		t.Fatalf("items[1]=%#v want mapped delete notification", items[1])
	}
}

func TestListNotificationsAutoCleansEnableDisableNotifications(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	client, roundTripper, transmitter := newTestNotificationClientWithTransmitter(
		[]*sgp22.NotificationMetadata{
			{SequenceNumber: 9, ProfileManagementOperation: sgp22.NotificationEventDelete, ICCID: iccid, Address: "delete.example.com"},
			{SequenceNumber: 12, ProfileManagementOperation: sgp22.NotificationEventEnable, ICCID: iccid, Address: "enable.example.com"},
			{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"},
			{SequenceNumber: 13, ProfileManagementOperation: sgp22.NotificationEventDisable, ICCID: iccid, Address: "disable.example.com"},
		},
		map[sgp22.SequenceNumber][]*sgp22.PendingNotification{
			12: {testPendingNotification(12, sgp22.NotificationEventEnable, iccid, "enable.example.com")},
			13: {testPendingNotification(13, sgp22.NotificationEventDisable, iccid, "disable.example.com")},
		},
		nil,
		nil,
		nil,
	)
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		return client, nil
	}, nil, nil, nil)
	mgr.readQueueWaitTimeout = 200 * time.Millisecond

	items, err := mgr.ListNotifications("A0000005591010FFFFFFFF8900000100")
	if err != nil {
		t.Fatalf("ListNotifications() error=%v", err)
	}
	if got := fmt.Sprint(roundTripper.handledHosts); got != "[enable.example.com disable.example.com]" {
		t.Fatalf("handledHosts=%v want [enable.example.com disable.example.com]", roundTripper.handledHosts)
	}
	if got := fmt.Sprint(transmitter.retrieved); got != "[12 13]" {
		t.Fatalf("retrieved=%v want [12 13]", transmitter.retrieved)
	}
	if got := fmt.Sprint(transmitter.removed); got != "[12 13]" {
		t.Fatalf("removed=%v want [12 13]", transmitter.removed)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d want 2 after cleanup", len(items))
	}
	if items[0].SequenceNumber != 11 || items[0].Event != "install" {
		t.Fatalf("items[0]=%#v want install notification", items[0])
	}
	if items[1].SequenceNumber != 9 || items[1].Event != "delete" {
		t.Fatalf("items[1]=%#v want delete notification", items[1])
	}
}

func TestListNotificationsKeepsVisibleItemsWhenAutoCleanupFails(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	client, roundTripper, transmitter := newTestNotificationClientWithTransmitter(
		[]*sgp22.NotificationMetadata{
			{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"},
			{SequenceNumber: 12, ProfileManagementOperation: sgp22.NotificationEventEnable, ICCID: iccid, Address: "enable.example.com"},
		},
		map[sgp22.SequenceNumber][]*sgp22.PendingNotification{
			12: {testPendingNotification(12, sgp22.NotificationEventEnable, iccid, "enable.example.com")},
		},
		map[sgp22.SequenceNumber]error{12: errors.New("retrieve failed")},
		nil,
		nil,
	)
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		return client, nil
	}, nil, nil, nil)
	mgr.readQueueWaitTimeout = 200 * time.Millisecond

	items, err := mgr.ListNotifications("A0000005591010FFFFFFFF8900000100")
	if err != nil {
		t.Fatalf("ListNotifications() error=%v", err)
	}
	if got := fmt.Sprint(transmitter.retrieved); got != "[12 12 12 12]" {
		t.Fatalf("retrieved=%v want cleanup retries for sequence 12", transmitter.retrieved)
	}
	if len(roundTripper.handledHosts) != 0 {
		t.Fatalf("handledHosts=%v want none when retrieve fails", roundTripper.handledHosts)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d want original visible notifications when cleanup fails", len(items))
	}
	if items[0].SequenceNumber != 12 || items[0].Event != "enable" {
		t.Fatalf("items[0]=%#v want failed cleanup notification to remain visible", items[0])
	}
	if items[1].SequenceNumber != 11 || items[1].Event != "install" {
		t.Fatalf("items[1]=%#v want install notification preserved", items[1])
	}
}

func TestListNotificationsWithoutAIDReadsStaticAIDsUnderReadArbitration(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}

	seenAIDs := make([]string, 0, 2)
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		got := strings.ToUpper(hex.EncodeToString(aid))
		seenAIDs = append(seenAIDs, got)
		switch got {
		case strings.ToUpper(hex.EncodeToString(AIDs[0])):
			client, _ := newTestNotificationClient([]*sgp22.NotificationMetadata{{
				SequenceNumber:             9,
				ProfileManagementOperation: sgp22.NotificationEventDelete,
				ICCID:                      iccid,
				Address:                    "delete.example.com",
			}}, nil, nil, nil)
			return client, nil
		case strings.ToUpper(hex.EncodeToString(AIDs[1])):
			client, _ := newTestNotificationClient([]*sgp22.NotificationMetadata{{
				SequenceNumber:             11,
				ProfileManagementOperation: sgp22.NotificationEventInstall,
				ICCID:                      iccid,
				Address:                    "install.example.com",
			}}, nil, nil, nil)
			return client, nil
		default:
			return nil, fmt.Errorf("unsupported AID %s", got)
		}
	}, nil, nil, nil)
	var waitCalled atomic.Int32
	mgr.apduArbiter = fakeAPDUIdleWaiter{wait: func(ctx context.Context) error {
		waitCalled.Add(1)
		return nil
	}}
	mgr.readQueueWaitTimeout = 200 * time.Millisecond

	items, err := mgr.ListNotifications("")
	if err != nil {
		t.Fatalf("ListNotifications() error=%v", err)
	}
	if waitCalled.Load() != 1 {
		t.Fatalf("WaitIdle calls=%d want 1", waitCalled.Load())
	}
	if got, want := seenAIDs, aidHexList(AIDs); !reflect.DeepEqual(got, want) {
		t.Fatalf("seenAIDs=%v want full static AIDs in order %v", got, want)
	}
	if len(items) != 2 {
		t.Fatalf("len(items)=%d want 2", len(items))
	}
	if items[0].SequenceNumber != 11 || items[0].AIDHex != strings.ToUpper(hex.EncodeToString(AIDs[1])) {
		t.Fatalf("items[0]=%#v want highest-sequence item from second AID", items[0])
	}
	if items[1].SequenceNumber != 9 || items[1].AIDHex != strings.ToUpper(hex.EncodeToString(AIDs[0])) {
		t.Fatalf("items[1]=%#v want lower-sequence item from first AID", items[1])
	}
}

func TestListNotificationsWithoutAIDAutoCleansStaticAIDNotifications(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}

	var cleanedRoundTripper *fakeNotificationRoundTripper
	var cleanedTransmitter *fakeNotificationTransmitter
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		got := strings.ToUpper(hex.EncodeToString(aid))
		switch got {
		case strings.ToUpper(hex.EncodeToString(AIDs[0])):
			client, rt, tx := newTestNotificationClientWithTransmitter(
				[]*sgp22.NotificationMetadata{{
					SequenceNumber:             21,
					ProfileManagementOperation: sgp22.NotificationEventEnable,
					ICCID:                      iccid,
					Address:                    "enable.example.com",
				}},
				map[sgp22.SequenceNumber][]*sgp22.PendingNotification{
					21: {testPendingNotification(21, sgp22.NotificationEventEnable, iccid, "enable.example.com")},
				},
				nil,
				nil,
				nil,
			)
			cleanedRoundTripper = rt
			cleanedTransmitter = tx
			return client, nil
		case strings.ToUpper(hex.EncodeToString(AIDs[1])):
			client, _ := newTestNotificationClient([]*sgp22.NotificationMetadata{{
				SequenceNumber:             11,
				ProfileManagementOperation: sgp22.NotificationEventInstall,
				ICCID:                      iccid,
				Address:                    "install.example.com",
			}}, nil, nil, nil)
			return client, nil
		default:
			return nil, fmt.Errorf("unsupported AID %s", got)
		}
	}, nil, nil, nil)
	mgr.readQueueWaitTimeout = 200 * time.Millisecond

	items, err := mgr.ListNotifications("")
	if err != nil {
		t.Fatalf("ListNotifications() error=%v", err)
	}
	if got := fmt.Sprint(cleanedRoundTripper.handledHosts); got != "[enable.example.com]" {
		t.Fatalf("handledHosts=%v want [enable.example.com]", cleanedRoundTripper.handledHosts)
	}
	if got := fmt.Sprint(cleanedTransmitter.removed); got != "[21]" {
		t.Fatalf("removed=%v want [21]", cleanedTransmitter.removed)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d want only install notification after cleanup", len(items))
	}
	if items[0].SequenceNumber != 11 || items[0].Event != "install" || items[0].AIDHex != strings.ToUpper(hex.EncodeToString(AIDs[1])) {
		t.Fatalf("items[0]=%#v want install notification from second AID", items[0])
	}
}

func TestRetryNotificationHandlesPendingNotificationBySequence(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	pending := &sgp22.PendingNotification{
		PendingNotification: bertlv.NewValue(bertlv.ContextSpecific.Primitive(0), []byte{0x01}),
		Notification:        &sgp22.NotificationMetadata{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"},
	}
	client, roundTripper := newTestNotificationClient(
		[]*sgp22.NotificationMetadata{{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"}},
		map[sgp22.SequenceNumber][]*sgp22.PendingNotification{11: {pending}},
		nil,
		nil,
	)
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		return client, nil
	}, nil, nil, nil)

	if err := mgr.RetryNotification(11, ""); err != nil {
		t.Fatalf("RetryNotification() error=%v", err)
	}
	if len(roundTripper.handledHosts) != 1 || roundTripper.handledHosts[0] != "install.example.com" {
		t.Fatalf("handledHosts=%v want [install.example.com]", roundTripper.handledHosts)
	}
}

func TestRetryNotificationClosesRealLPAClientOnSuccess(t *testing.T) {
	ch := &lifecycleSmartCardChannel{response: lifecycleRetrieveNotificationResponse(t)}
	var roundTripper *fakeNotificationRoundTripper
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		client, rt := newLifecycleNotificationClient(t, ch, nil)
		roundTripper = rt
		return client, nil
	}, nil, nil, nil)

	if err := mgr.RetryNotification(11, ""); err != nil {
		t.Fatalf("RetryNotification() error=%v", err)
	}
	if len(roundTripper.handledHosts) != 1 || roundTripper.handledHosts[0] != "install.example.com" {
		t.Fatalf("handledHosts=%v want [install.example.com]", roundTripper.handledHosts)
	}
	if ch.closeCalls.Load() != 1 || ch.disconnectCalls.Load() != 1 {
		t.Fatalf("closeCalls=%d disconnectCalls=%d want 1/1", ch.closeCalls.Load(), ch.disconnectCalls.Load())
	}
	if ch.closedChannel.Load() != 1 {
		t.Fatalf("closedChannel=%d want 1", ch.closedChannel.Load())
	}
}

func TestRetryNotificationClosesRealLPAClientOnRetrieveFailure(t *testing.T) {
	ch := &lifecycleSmartCardChannel{response: emptyRetrieveNotificationResponse()}
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		client, _ := newLifecycleNotificationClient(t, ch, nil)
		return client, nil
	}, nil, nil, nil)

	if err := mgr.RetryNotification(11, ""); err == nil {
		t.Fatal("RetryNotification() error=nil, want retrieve failure")
	}
	if ch.closeCalls.Load() != 1 || ch.disconnectCalls.Load() != 1 {
		t.Fatalf("closeCalls=%d disconnectCalls=%d want 1/1", ch.closeCalls.Load(), ch.disconnectCalls.Load())
	}
}

func TestCloseLPAClientReportsCloseFailuresAndRecoversPanics(t *testing.T) {
	closeErr := errors.New("close failed")
	ch := &lifecycleSmartCardChannel{response: emptyRetrieveNotificationResponse(), closeErr: closeErr}
	client, _ := newLifecycleNotificationClient(t, ch, nil)

	if err := closeLPAClient(client); !errors.Is(err, closeErr) {
		t.Fatalf("closeLPAClient() error=%v want %v", err, closeErr)
	}
	if ch.closeCalls.Load() != 1 {
		t.Fatalf("closeCalls=%d want 1", ch.closeCalls.Load())
	}
	if ch.disconnectCalls.Load() != 0 {
		t.Fatalf("disconnectCalls=%d want 0 when close fails", ch.disconnectCalls.Load())
	}

	if err := closeLPAClient(&lpa.Client{}); err != nil {
		t.Fatalf("closeLPAClient() recovered panic error=%v want nil", err)
	}
}

func TestExpectedPostResetLPAClientCloseError(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		err       error
		want      bool
	}{
		{
			name:      "switch pre refresh close logical channel",
			operation: "switch_profile_pre_refresh",
			err:       errors.New("qmi_uim_card_reset: close logical channel"),
			want:      true,
		},
		{
			name:      "switch deferred close logical channel",
			operation: "switch_profile_deferred",
			err:       errors.New("close logical channel: QMI error: service=0x0b msg=0x003f result=0x0001 error=0x0030"),
			want:      true,
		},
		{
			name:      "disable pre refresh close logical channel",
			operation: "disable_profile_pre_refresh",
			err:       errors.New("close logical channel: QMI error: service=0x0b msg=0x003f result=0x0001 error=0x0030"),
			want:      true,
		},
		{
			name:      "download close should remain warn",
			operation: "download_profile_deferred",
			err:       errors.New("close logical channel failed"),
			want:      false,
		},
		{
			name:      "switch unrelated error should remain warn",
			operation: "switch_profile_deferred",
			err:       errors.New("transport timeout"),
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpectedPostResetLPAClientCloseError(tt.operation, tt.err); got != tt.want {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestRetryNotificationClassifiesRetrieveAndHandleFailures(t *testing.T) {
	iccid, err := sgp22.NewICCID("8986001234567890123")
	if err != nil {
		t.Fatalf("NewICCID() error=%v", err)
	}
	pending := &sgp22.PendingNotification{
		PendingNotification: bertlv.NewValue(bertlv.ContextSpecific.Primitive(0), []byte{0x01}),
		Notification:        &sgp22.NotificationMetadata{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"},
	}
	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		client, _ := newTestNotificationClient(
			[]*sgp22.NotificationMetadata{{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"}},
			map[sgp22.SequenceNumber][]*sgp22.PendingNotification{11: {pending}},
			map[sgp22.SequenceNumber]error{11: errors.New("retrieve failed")},
			nil,
		)
		return client, nil
	}, nil, nil, nil)

	if err := mgr.RetryNotification(11, ""); err == nil {
		t.Fatal("RetryNotification() error=nil, want retrieve failure")
	}

	mgr = newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		client, _ := newTestNotificationClient(
			[]*sgp22.NotificationMetadata{{SequenceNumber: 11, ProfileManagementOperation: sgp22.NotificationEventInstall, ICCID: iccid, Address: "install.example.com"}},
			map[sgp22.SequenceNumber][]*sgp22.PendingNotification{11: {pending}},
			nil,
			map[string]error{"install.example.com": errors.New("handle failed")},
		)
		return client, nil
	}, nil, nil, nil)

	if err := mgr.RetryNotification(11, ""); err == nil {
		t.Fatal("RetryNotification() error=nil, want handle failure")
	}
}

func TestDoForEachEUICCReattemptsAfterPartialFailure(t *testing.T) {
	// Use eSTK AID pairs so shouldContinueAIDScanAfterSuccess allows continuing
	aid1 := AIDs[0] // eSTK.me SE0
	aid2 := AIDs[1] // eSTK.me SE1
	attempts := make(map[string]int)

	mgr := newManagerWithChannelFactory("dev-esim", func(aid []byte) (*lpa.Client, error) {
		aidHex := strings.ToUpper(hex.EncodeToString(aid))
		attempts[aidHex]++
		if bytes.Equal(aid, aid1) {
			return &lpa.Client{APDU: fakeProfileOperationTransmitter{
				eid: []byte{0x89, 0x04, 0x40, 0x45, 0x84, 0x67, 0x27, 0x49},
			}}, nil
		}
		if bytes.Equal(aid, aid2) {
			return nil, errors.New("transient failure on aid2 EID")
		}
		return nil, errors.New("unknown AID")
	}, nil, nil, nil)
	mgr.closeClient = func(client *lpa.Client) error { return nil }

	foundAny, err := mgr.doForEachEUICC([][]byte{aid1, aid2}, func(client *lpa.Client, aid []byte, eidStr string) error {
		return nil
	})

	if !foundAny {
		t.Fatal("doForEachEUICC returned foundAny=false, want true (aid1 succeeded)")
	}
	// Even with partial failure, returns success since we found at least one eUICC
	if err != nil {
		t.Fatalf("doForEachEUICC returned err=%v, want nil (found aid1 despite aid2 failure)", err)
	}

	attempts = make(map[string]int)
	foundAny2, _ := mgr.doForEachEUICC([][]byte{aid1, aid2}, func(client *lpa.Client, aid []byte, eidStr string) error {
		return nil
	})
	if !foundAny2 {
		t.Fatal("second scan returned foundAny=false")
	}
	if attempts[strings.ToUpper(hex.EncodeToString(aid1))] == 0 {
		t.Fatal("second scan did not re-attempt aid1")
	}
}

func TestHasReusableChipProductInfoIgnoresFallbackFirmware(t *testing.T) {
	// Firmware alone must NOT count as reusable eSTK product info: when the
	// eSTK.me Product AID query fails (e.g. logical channel resource exhaustion),
	// info.Firmware is still populated by the standard EUICCInfo2 fallback.
	// If firmware-only cache were treated as reusable, parseESTKmeInfo would be
	// permanently skipped and SkuName/SerialNumber never retrieved.
	if hasReusableChipProductInfo(&EUICCChipInfo{Firmware: "37.1.41"}) {
		t.Fatal("hasReusableChipProductInfo(firmware-only)=true, want false (firmware is pollutable by EUICCInfo2 fallback)")
	}
	if hasReusableChipProductInfo(&EUICCChipInfo{}) {
		t.Fatal("hasReusableChipProductInfo(empty)=true, want false")
	}
	if hasReusableChipProductInfo(nil) {
		t.Fatal("hasReusableChipProductInfo(nil)=true, want false")
	}
	if !hasReusableChipProductInfo(&EUICCChipInfo{SkuName: "eSTK.me Max"}) {
		t.Fatal("hasReusableChipProductInfo(sku set)=false, want true")
	}
	if !hasReusableChipProductInfo(&EUICCChipInfo{SerialNumber: "T3VAMD0"}) {
		t.Fatal("hasReusableChipProductInfo(serial set)=false, want true")
	}
}
