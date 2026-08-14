package backend

import (
	"context"
	"testing"
	"time"

	"github.com/WongLoki/DJ4Hub/pkg/mbim"
)

func TestMBIMBackendUSSDProviderConformance(t *testing.T) {
	var _ USSDProvider = (*MBIMBackend)(nil)
}

func TestMBIMBackendExecuteUSSDMapsResult(t *testing.T) {
	src := &fakeMBIMSource{
		ussdResult: mbim.USSDResult{
			Response: mbim.USSDRespNoActionRequired,
			DCS:      0x0F,
			Text:     "balance 10",
			RawHex:   "c2f29b0c4a87e520f4",
		},
	}
	b := NewMBIMBackend("", src)

	got, err := b.ExecuteUSSD(context.Background(), "*100#", 2*time.Second)
	if err != nil {
		t.Fatalf("ExecuteUSSD: %v", err)
	}
	if got.Status != int(mbim.USSDRespNoActionRequired) || got.DCS != 0x0F {
		t.Fatalf("ExecuteUSSD() = %+v", got)
	}
	if got.Text != "balance 10" || got.RawText != "c2f29b0c4a87e520f4" {
		t.Fatalf("ExecuteUSSD() = %+v, want mapped text/raw hex", got)
	}
	if src.ussdCommand != "*100#" || src.ussdTimeout != 2*time.Second {
		t.Fatalf("source saw command=%q timeout=%v", src.ussdCommand, src.ussdTimeout)
	}
}

func TestMBIMBackendContinueUSSDMapsResult(t *testing.T) {
	src := &fakeMBIMSource{
		ussdResult: mbim.USSDResult{
			Response: mbim.USSDRespNoActionRequired,
			DCS:      0x0F,
			Text:     "continued",
			RawHex:   "e8329bfd06",
		},
	}
	b := NewMBIMBackend("", src)

	got, err := b.ContinueUSSD(context.Background(), "1", 3*time.Second)
	if err != nil {
		t.Fatalf("ContinueUSSD: %v", err)
	}
	if got.Status != int(mbim.USSDRespNoActionRequired) || got.DCS != 0x0F {
		t.Fatalf("ContinueUSSD() = %+v", got)
	}
	if got.Text != "continued" || got.RawText != "e8329bfd06" {
		t.Fatalf("ContinueUSSD() = %+v, want mapped text/raw hex", got)
	}
	if src.ussdContinueInput != "1" || src.ussdContinueTimeout != 3*time.Second {
		t.Fatalf("source saw input=%q timeout=%v", src.ussdContinueInput, src.ussdContinueTimeout)
	}
}

func TestMBIMBackendCancelUSSDDelegates(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)

	if err := b.CancelUSSD(context.Background()); err != nil {
		t.Fatalf("CancelUSSD: %v", err)
	}
	if !src.cancelUSSD {
		t.Fatal("CancelUSSD did not delegate to MBIM source")
	}
}
