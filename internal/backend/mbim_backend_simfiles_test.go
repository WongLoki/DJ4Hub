package backend

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/WongLoki/dj-4g-hub/pkg/mbim"
)

func mbimResetCapableForTest() *mbim.Capabilities {
	return &mbim.Capabilities{
		Services: mbim.DeviceServices{Elements: []mbim.DeviceServiceElement{
			{
				Service: mbim.UUIDMSBasicConnectExtensions,
				CIDs:    []uint32{mbim.CIDMSBasicConnectExtDeviceReset},
			},
		}},
	}
}

func TestMBIMRebootIssuesDeviceReset(t *testing.T) {
	src := &fakeMBIMSource{capability: mbimResetCapableForTest()}
	b := NewMBIMBackend("", src)
	if err := b.Reboot(context.Background()); err != nil {
		t.Fatalf("Reboot() error = %v", err)
	}
	if !src.resetCalled {
		t.Fatal("Reboot() did not call DeviceReset")
	}
}

func TestMBIMRebootPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	src := &fakeMBIMSource{capability: mbimResetCapableForTest(), resetErr: wantErr}
	b := NewMBIMBackend("", src)
	if err := b.Reboot(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Reboot() error = %v, want %v", err, wantErr)
	}
}

func TestMBIMRebootRequiresDeviceResetCapability(t *testing.T) {
	src := &fakeMBIMSource{}
	b := NewMBIMBackend("", src)

	err := b.Reboot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Reboot() error = %v, want not supported", err)
	}
	if src.resetCalled {
		t.Fatal("Reboot() called DeviceReset without advertised reset capability")
	}
}

func TestMBIMGetNativeSPNDecodesEFSPN(t *testing.T) {
	src := &fakeMBIMSource{}
	src.efFn = func(fileID uint16) ([]byte, error) {
		if fileID == 0x6F46 {
			// byte0 display flag, then ASCII "CMCC".
			return []byte{0x01, 'C', 'M', 'C', 'C', 0xFF, 0xFF}, nil
		}
		return nil, nil
	}
	b := NewMBIMBackend("", src)

	got, err := b.GetNativeSPN(context.Background())
	if err != nil {
		t.Fatalf("GetNativeSPN() error = %v", err)
	}
	if got != "CMCC" {
		t.Fatalf("GetNativeSPN() = %q, want CMCC", got)
	}
}

func TestMBIMGetNativeSPNEmptyWhenEFEmpty(t *testing.T) {
	src := &fakeMBIMSource{}
	src.efFn = func(uint16) ([]byte, error) { return nil, nil }
	b := NewMBIMBackend("", src)

	got, err := b.GetNativeSPN(context.Background())
	if err != nil {
		t.Fatalf("GetNativeSPN() error = %v", err)
	}
	if got != "" {
		t.Fatalf("GetNativeSPN() = %q, want empty", got)
	}
}

func mbimSubscriberWithIMSI(imsi string) mbim.SubscriberReady {
	return mbim.SubscriberReady{IMSI: imsi, ReadyState: 1}
}

func TestMBIMGetSIMMetadataFillsGIDAndMNCLength(t *testing.T) {
	src := &fakeMBIMSource{
		sub: mbimSubscriberWithIMSI("460001234567890"),
	}
	src.efFn = func(fileID uint16) ([]byte, error) {
		switch fileID {
		case efAD:
			// EF_AD: byte0..2 admin, byte3 = MNC length (2).
			return []byte{0x00, 0x00, 0x00, 0x02}, nil
		case efGID1:
			return []byte{0xA1, 0xB2}, nil
		case efGID2:
			return []byte{0xC3}, nil
		default:
			return nil, nil
		}
	}
	b := NewMBIMBackend("", src)

	meta, err := b.GetSIMMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetSIMMetadata() error = %v", err)
	}
	if meta == nil {
		t.Fatal("GetSIMMetadata() = nil")
	}
	if meta.NativeMCC != "460" || meta.NativeMNC != "00" {
		t.Fatalf("MCC/MNC = %s/%s", meta.NativeMCC, meta.NativeMNC)
	}
	if hex.EncodeToString([]byte{0xA1, 0xB2}) != meta.GID1 {
		t.Fatalf("GID1 = %q", meta.GID1)
	}
	if meta.GID2 != "c3" {
		t.Fatalf("GID2 = %q, want c3", meta.GID2)
	}
}

func TestMBIMGetSIMMetadataFillsPNNOPLAndUST(t *testing.T) {
	src := &fakeMBIMSource{
		sub: mbimSubscriberWithIMSI("460001234567890"),
	}
	src.efFn = func(fileID uint16) ([]byte, error) {
		switch fileID {
		case efUST:
			// Enable service 1 and 2 (byte 1 = 0x03)
			return []byte{0x03, 0x00, 0x00}, nil
		default:
			return nil, nil
		}
	}
	src.efRecFn = func(fileID uint16, rec uint32) ([]byte, error) {
		if fileID == efPNN {
			if rec == 1 {
				data := append([]byte{0x43, 0x04}, []byte("TEST")...)
				return append(data, 0xFF, 0xFF), nil
			}
		}
		if fileID == efOPL {
			if rec == 1 {
				// Mock OPL Record 1: PLMN 46000, PNN ID 1.
				// Format: PLMN (3 bytes), LAC Start (2), LAC End (2), PNN ID (1)
				// 46000 = 64 00 00
				return []byte{0x64, 0xF0, 0x00, 0x00, 0x00, 0xFF, 0xFE, 0x01, 0xFF}, nil
			}
		}
		return nil, errors.New("not found")
	}
	b := NewMBIMBackend("", src)

	meta, err := b.GetSIMMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetSIMMetadata() error = %v", err)
	}
	if meta.ServiceTable == nil || meta.ServiceTable.Kind != "UST" {
		t.Fatalf("ServiceTable = %+v", meta.ServiceTable)
	}
	if len(meta.PNN) != 1 {
		t.Fatalf("PNN = %+v, want len=1", meta.PNN)
	}
	if meta.PNN[0].FullName != "TEST" {
		t.Fatalf("PNN[0].FullName = %q, want TEST", meta.PNN[0].FullName)
	}
	if len(meta.OPL) != 1 {
		t.Fatalf("OPL = %+v, want len=1", meta.OPL)
	}
	if meta.OPL[0].PLMN != "46000" || meta.OPL[0].LACStart != 0 || meta.OPL[0].LACEnd != 0xFFFE || meta.OPL[0].PNNRecord != 1 {
		t.Fatalf("OPL[0] = %+v, want PLMN=46000 LAC=0-65534 PNNRecord=1", meta.OPL[0])
	}
}

func TestMBIMGetMSISDNFallsBackToEF(t *testing.T) {
	src := &fakeMBIMSource{} // SubscriberReady.MSISDN empty by default
	src.efFn = func(fileID uint16) ([]byte, error) {
		if fileID == efMSISDN {
			// EF_MSISDN record tail: ... [len][TON/NPI][dialing BCD...]
			// length=0x07, TON/NPI=0x91 (international), number 8613800100500.
			return buildEFMSISDNRecord("8613800100500"), nil
		}
		return nil, nil
	}
	b := NewMBIMBackend("", src)

	got, err := b.GetMSISDN(context.Background())
	if err != nil {
		t.Fatalf("GetMSISDN() error = %v", err)
	}
	if got != "8613800100500" {
		t.Fatalf("GetMSISDN() = %q, want 8613800100500", got)
	}
}

// buildEFMSISDNRecord builds a minimal EF_MSISDN record with the dialing number
// stored as swapped BCD in the standard 14-byte trailer.
func buildEFMSISDNRecord(number string) []byte {
	// Layout (TS 51.011 EF_MSISDN): [Alpha...][Length(1)][TON/NPI(1)][Dialing(10)][CCP(1)][Ext(1)]
	// We emit only the 14-byte trailer (alpha identifier length 0).
	rec := make([]byte, 14)
	bcd := encodeSwappedBCDForTest(number)
	rec[0] = byte(len(bcd) + 1) // length of BCD + TON/NPI byte
	rec[1] = 0x91               // international
	copy(rec[2:12], bcd)
	for i := 2 + len(bcd); i < 12; i++ {
		rec[i] = 0xFF
	}
	rec[12], rec[13] = 0xFF, 0xFF
	return rec
}

func encodeSwappedBCDForTest(num string) []byte {
	digits := []byte(num)
	out := make([]byte, (len(digits)+1)/2)
	for i := 0; i < len(digits); i++ {
		nib := digits[i] - '0'
		if i%2 == 0 {
			out[i/2] = nib
		} else {
			out[i/2] |= nib << 4
		}
	}
	if len(digits)%2 == 1 {
		out[len(out)-1] |= 0xF0
	}
	return out
}
