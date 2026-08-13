package backend

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/WongLoki/dj-4g-hub/internal/modem"
)

// SIM EF file identifiers (3GPP TS 31.102 / 51.011).
const (
	efSPN    uint16 = 0x6F46
	efAD     uint16 = 0x6FAD
	efGID1   uint16 = 0x6F3E
	efGID2   uint16 = 0x6F3F
	efPNN    uint16 = 0x6FC5
	efOPL    uint16 = 0x6FC6
	efUST    uint16 = 0x6F38 // USIM Service Table
	efMSISDN uint16 = 0x6F40
)

// readNativeSPN reads and decodes EF_SPN via the MBIM UICC channel.
func (b *MBIMBackend) readNativeSPN(ctx context.Context) (string, error) {
	data, err := b.source.ReadSIMEF(ctx, efSPN, 0)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}
	spn, derr := modem.DecodeEFSPN(data)
	if derr != nil {
		return "", nil // unreadable SPN is not a hard error
	}
	return spn, nil
}

func (b *MBIMBackend) readSIMMetadata(ctx context.Context) (*SIMMetadata, error) {
	mcc, mnc, err := b.GetNativeMCCMNC(ctx)
	if err != nil {
		return nil, err
	}
	meta := &SIMMetadata{NativeMCC: mcc, NativeMNC: mnc}

	if ad, _ := b.source.ReadSIMEF(ctx, efAD, 0); len(ad) >= 4 {
		mncLen := int(ad[3])
		if mncLen == 2 || mncLen == 3 {
			if len(mnc) > mncLen {
				meta.NativeMNC = mnc[:mncLen]
			}
		}
	}
	if gid1, _ := b.source.ReadSIMEF(ctx, efGID1, 0); len(gid1) > 0 {
		meta.GID1 = hex.EncodeToString(trimFF(gid1))
	}
	if gid2, _ := b.source.ReadSIMEF(ctx, efGID2, 0); len(gid2) > 0 {
		meta.GID2 = hex.EncodeToString(trimFF(gid2))
	}

	var pnn []PNNRecord
	for i := 1; ; i++ {
		rec, err := b.source.ReadSIMRecordEF(ctx, efPNN, uint32(i))
		if err != nil || len(rec) == 0 {
			break
		}
		if p, ok := modem.DecodePNNRecord(i, rec); ok {
			pnn = append(pnn, PNNRecord(p))
		}
	}
	if len(pnn) > 0 {
		meta.PNN = pnn
	}

	var opl []OPLRecord
	for i := 1; ; i++ {
		rec, err := b.source.ReadSIMRecordEF(ctx, efOPL, uint32(i))
		if err != nil || len(rec) == 0 {
			break
		}
		if o, ok := modem.DecodeOPLRecord(i, rec); ok {
			opl = append(opl, OPLRecord(o))
		}
	}
	if len(opl) > 0 {
		meta.OPL = opl
	}

	if ust, _ := b.source.ReadSIMEF(ctx, efUST, 0); len(ust) > 0 {
		if st := modem.DecodeSIMServiceTable("UST", ust); st != nil {
			meta.ServiceTable = &SIMServiceTable{
				Kind:            st.Kind,
				RawHex:          st.RawHex,
				EnabledServices: append([]int(nil), st.EnabledServices...),
			}
		}
	}

	return meta, nil
}

// trimFF drops trailing 0xFF padding bytes from a raw EF read.
func trimFF(b []byte) []byte {
	n := len(b)
	for n > 0 && b[n-1] == 0xFF {
		n--
	}
	return b[:n]
}

// readMSISDNFromEF reads EF_MSISDN and decodes the dialing number.
func (b *MBIMBackend) readMSISDNFromEF(ctx context.Context) string {
	rec, err := b.source.ReadSIMEF(ctx, efMSISDN, 0)
	if err != nil || len(rec) < 14 {
		return ""
	}
	// The 14-byte trailer is at the end of the record.
	tail := rec[len(rec)-14:]
	length := int(tail[0])
	if length < 1 || length > 11 {
		return ""
	}
	// tail[1] = TON/NPI; dialing BCD is tail[2 : 2+(length-1)].
	bcd := tail[2 : 2+(length-1)]
	number := modem.DecodeSwappedBCD(bcd)
	return strings.TrimRight(number, "f")
}
