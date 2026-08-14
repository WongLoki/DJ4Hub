package backend

import (
	"context"
	"testing"

	"github.com/WongLoki/DJ4Hub/pkg/mbim"
)

func TestMBIMBackendImplementsOperatorSelectionProvider(t *testing.T) {
	var _ OperatorSelectionProvider = (*MBIMBackend)(nil)
}

func TestMBIMBackendScanOperatorsMapsVisibleProviders(t *testing.T) {
	src := &fakeMBIMSource{providers: []mbim.Provider{{
		PLMN:          "310260",
		Name:          "T-Mobile",
		State:         1<<4 | 1<<3,
		CellularClass: mbim.CellularClassGSM,
		RSSI:          17,
	}}}
	b := NewMBIMBackend("", src)

	candidates, err := b.ScanOperators(context.Background())
	if err != nil {
		t.Fatalf("ScanOperators: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d, want 1", len(candidates))
	}
	got := candidates[0]
	if got.PLMN != "310260" || got.MCC != "310" || got.MNC != "260" || got.OperatorName != "T-Mobile" {
		t.Fatalf("candidate identity = %+v", got)
	}
	if got.Status != "current" {
		t.Fatalf("status = %q, want current", got.Status)
	}
	if len(got.RATs) != 1 || got.RATs[0] != OperatorRATGSM {
		t.Fatalf("RATs = %+v, want gsm", got.RATs)
	}
}

func TestMBIMBackendSetOperatorSelectionManualDelegatesRegisterSet(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)

	sel, err := b.SetOperatorSelection(context.Background(), SetOperatorSelectionRequest{
		Mode: OperatorSelectionManual,
		PLMN: "310260",
		RAT:  OperatorRATGSM,
	})
	if err != nil {
		t.Fatalf("SetOperatorSelection: %v", err)
	}
	if src.setRegisterAction != mbim.RegisterActionManual || src.setRegisterPLMN != "310260" {
		t.Fatalf("set register = action:%d plmn:%q", src.setRegisterAction, src.setRegisterPLMN)
	}
	if sel.Mode != OperatorSelectionManual || sel.PLMN != "310260" || sel.RAT != OperatorRATGSM {
		t.Fatalf("selection = %+v", sel)
	}
}

func TestMBIMBackendSetOperatorSelectionAutomaticDelegatesRegisterSet(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)

	sel, err := b.SetOperatorSelection(context.Background(), SetOperatorSelectionRequest{Mode: OperatorSelectionAutomatic})
	if err != nil {
		t.Fatalf("SetOperatorSelection: %v", err)
	}
	if src.setRegisterAction != mbim.RegisterActionAutomatic || src.setRegisterPLMN != "" {
		t.Fatalf("set register = action:%d plmn:%q", src.setRegisterAction, src.setRegisterPLMN)
	}
	if sel.Mode != OperatorSelectionAutomatic {
		t.Fatalf("selection = %+v, want automatic", sel)
	}
}

func TestMBIMBackendGetOperatorSelectionMapsRegisterState(t *testing.T) {
	src := &fakeMBIMSource{reg: mbim.RegisterState{
		RegisterMode: mbim.RegisterModeManual,
		ProviderID:   "46000",
		ProviderName: "CMCC",
		MCC:          "460",
		MNC:          "00",
	}}
	b := NewMBIMBackend("", src)

	sel, err := b.GetOperatorSelection(context.Background())
	if err != nil {
		t.Fatalf("GetOperatorSelection: %v", err)
	}
	if sel.Mode != OperatorSelectionManual || sel.PLMN != "46000" || sel.OperatorName != "CMCC" {
		t.Fatalf("selection = %+v", sel)
	}
}

func TestMBIMBackendGetOperatorSelectionMapsAutomaticRegisterMode(t *testing.T) {
	src := &fakeMBIMSource{reg: mbim.RegisterState{
		RegisterMode: mbim.RegisterModeAutomatic,
		ProviderID:   "310260",
		ProviderName: "T-Mobile",
		MCC:          "310",
		MNC:          "260",
	}}
	b := NewMBIMBackend("", src)

	sel, err := b.GetOperatorSelection(context.Background())
	if err != nil {
		t.Fatalf("GetOperatorSelection: %v", err)
	}
	if sel.Mode != OperatorSelectionAutomatic || sel.PLMN != "" {
		t.Fatalf("selection = %+v, want automatic without PLMN", sel)
	}
}
