package esim

import (
	"fmt"

	"github.com/WongLoki/dj-4g-hub/internal/esim/pki"
	"github.com/WongLoki/dj-4g-hub/pkg/logger"
	"github.com/damonto/euicc-go/bertlv"
	"github.com/damonto/euicc-go/bertlv/primitive"
	"github.com/damonto/euicc-go/lpa"
)

type euiccInfoReader interface {
	EUICCInfo2() (*bertlv.TLV, error)
	EUICCInfo1() (*bertlv.TLV, error)
	EUICCConfiguredAddresses() (*lpa.EUICCConfiguredAddresses, error)
}

func (m *Manager) enrichEUICCInfo(reader euiccInfoReader, euicc *EUICCInfo) {
	if reader == nil || euicc == nil {
		return
	}

	if tlv, err := reader.EUICCInfo2(); err == nil {
		applyEUICCInfoTLV(euicc, "euicc_info2", tlv)
	} else {
		euicc.InfoError = err.Error()
		logger.Warn("获取 EUICCInfo2 失败，尝试降级 EUICCInfo1",
			"device", m.deviceID,
			"EID", euicc.EID,
			"err", err)
		if tlv1, err1 := reader.EUICCInfo1(); err1 == nil {
			applyEUICCInfoTLV(euicc, "euicc_info1", tlv1)
		} else {
			logger.Debug("获取 EUICCInfo1 也失败",
				"device", m.deviceID,
				"EID", euicc.EID,
				"err", err1)
		}
	}

	if addresses, err := reader.EUICCConfiguredAddresses(); err == nil && addresses != nil {
		euicc.DefaultSMDPAddress = addresses.DefaultSMDPAddress
		euicc.RootSMDSAddress = addresses.RootSMDSAddress
	} else if err != nil {
		logger.Debug("获取 eUICC 配置地址失败",
			"device", m.deviceID,
			"EID", euicc.EID,
			"err", err)
	}
}

func applyEUICCInfoTLV(euicc *EUICCInfo, source string, tlv *bertlv.TLV) {
	if euicc == nil || tlv == nil {
		return
	}
	euicc.InfoSource = source
	switch tlv.Tag.Value() {
	case 32:
		euicc.InfoVersion = "1"
	case 34:
		euicc.InfoVersion = "2"
	}

	if resource := tlv.First(bertlv.ContextSpecific.Primitive(4)); resource != nil {
		data, _ := resource.MarshalBinary()
		if len(data) > 0 {
			data[0] = 0x30
			if err := resource.UnmarshalBinary(data); err == nil {
				if freeNvEntry := resource.First(bertlv.ContextSpecific.Primitive(2)); freeNvEntry != nil {
					primitive.UnmarshalInt(&euicc.FreeNvramBytes).UnmarshalBinary(freeNvEntry.Value)
					euicc.FreeNvram = formatBytes(int64(euicc.FreeNvramBytes))
				}
			} else {
				logger.Debug("解析 extResource 失败", "err", err)
			}
		}
	}

	if fwVer := tlv.First(bertlv.ContextSpecific.Primitive(3)); fwVer != nil {
		val := fwVer.Value
		if len(val) == 3 {
			euicc.Firmware = fmt.Sprintf("%d.%d.%d", val[0], val[1], val[2])
		}
	}

	if sasEntry := tlv.First(bertlv.Universal.Primitive(12)); sasEntry != nil {
		euicc.SASAccreditationNumber = string(sasEntry.Value)
	}

	if manufacturer := pki.LookupManufacturer(euicc.EID, euicc.SASAccreditationNumber); manufacturer != "" {
		euicc.Manufacturer = manufacturer
	}

	if ciList := tlv.First(bertlv.ContextSpecific.Constructed(10)); ciList != nil {
		var keyIDs [][]byte
		for _, child := range ciList.Children {
			keyIDs = append(keyIDs, child.Value)
		}
		euicc.Certificates = pki.LookupCertificateIssuers(keyIDs)
	}
}
