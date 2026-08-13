// Package pki 提供 eUICC 芯片的 PKI 公开数据查询能力
// 数据来源：https://euicc-manual.osmocom.org
// 使用 go:generate 更新内嵌的 JSON 字典：
//
//go:generate curl -sL -o ci.json https://euicc-manual.osmocom.org/docs/pki/ci/manifest.json
//go:generate curl -sL -o accredited.json https://euicc-manual.osmocom.org/docs/pki/eum/accredited.json
package pki

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/WongLoki/dj-4g-hub/pkg/logger"
)

//go:embed ci.json
var ciData []byte

//go:embed accredited.json
var accreditedData []byte

// CertificateIssuer eSIM 证书签发机构
type CertificateIssuer struct {
	KeyID   string `json:"key-id"`
	Country string `json:"country"`
	Name    string `json:"name"`
}

// Accredited 认证供应商字典
type Accredited struct {
	Version   uint8      `json:"version"`
	Suppliers []Supplier `json:"suppliers"`
}

// Supplier eUICC 芯片供应商
type Supplier struct {
	Name      string            `json:"name"`
	Abbr      string            `json:"abbr,omitempty"`
	Region    string            `json:"country"`
	EUM       []string          `json:"eum,omitempty"`
	Locations map[string]string `json:"locations"`
}

var (
	issuers []CertificateIssuer
	sites   Accredited
)

func init() {
	if err := json.Unmarshal(ciData, &issuers); err != nil {
		logger.Error("解析 CI 证书签发机构数据失败", "err", err)
	}
	if err := json.Unmarshal(accreditedData, &sites); err != nil {
		logger.Error("解析 Accredited 供应商数据失败", "err", err)
	}
}

// LookupCertificateIssuer 根据 keyID（hex 字符串）查找证书签发机构名称
func LookupCertificateIssuer(keyID string) string {
	for _, ci := range issuers {
		if strings.HasPrefix(keyID, ci.KeyID) {
			return ci.Name
		}
	}
	return keyID
}

// LookupCertificateIssuers 从 EUICCInfo2 中的 euiccCiPKIdListForSigning 字段批量查询
// 入参是原始二进制 keyID 列表，返回人类可读的签发机构名称列表
func LookupCertificateIssuers(keyIDs [][]byte) []string {
	result := make([]string, 0, len(keyIDs))
	for _, kid := range keyIDs {
		result = append(result, LookupCertificateIssuer(hex.EncodeToString(kid)))
	}
	return result
}

// LookupManufacturer 根据 EID 前 8 位（EUM 前缀）查找芯片制造商名称
// sasAccreditationNumber 可选，来自 EUICCInfo2 中的 sasAccreditationNumber 字段
// 返回格式如 "Kigen 🇬🇧" 或 "Thales 🇫🇷"
func LookupManufacturer(eid string, sasAccreditationNumber string) string {
	if len(eid) < 8 {
		return ""
	}
	eum := eid[:8]
	for _, supplier := range sites.Suppliers {
		if slices.Contains(supplier.EUM, eum) {
			flag := regionFlag(supplier.Region)
			fallback := fmt.Sprintf("%s %s", supplier.Name, flag)
			if len(sasAccreditationNumber) < 5 {
				return fallback
			}
			if value, ok := supplier.Locations[sasAccreditationNumber[:5]]; ok {
				return fmt.Sprintf("%s %s", supplier.Name, regionFlag(value))
			}
			return fallback
		}
	}
	return ""
}

// regionFlag 将两字母国家代码转换为 emoji 国旗
func regionFlag(code string) string {
	if len(code) < 2 {
		return ""
	}
	return string(0x1F1E6+rune(code[0])-'A') + string(0x1F1E6+rune(code[1])-'A')
}
