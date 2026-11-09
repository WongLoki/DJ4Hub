package modem

import (
	"testing"

	"github.com/WongLoki/DJ4Hub/internal/config"
)

// Fix B 红测：/api/status 报 ps_attached:false 而模块实际 +CGATT:1 ——
// AT 后端从未查询 AT+CGATT?，PS 附着状态永远是零值 false。
// 本文件锁定 PS 附着查询与解析的行为；修复前编译失败复现「查询不存在」。

func TestParseCGATT(t *testing.T) {
	tests := []struct {
		name   string
		resp   string
		want   bool
		wantOK bool
	}{
		{name: "attached", resp: "+CGATT: 1\r\n\r\nOK", want: true, wantOK: true},
		{name: "detached", resp: "+CGATT: 0\r\n\r\nOK", want: false, wantOK: true},
		{name: "extra whitespace", resp: "  +CGATT:   1  ", want: true, wantOK: true},
		{name: "embedded in unsolicited noise", resp: "\r\n+CREG: 2\r\n+CGATT: 1\r\nOK", want: true, wantOK: true},
		{name: "unparseable value", resp: "+CGATT: x", wantOK: false},
		{name: "wrong prefix", resp: "+CGACT: 1,1", wantOK: false},
		{name: "empty response", resp: "", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseCGATT(test.resp)
			if ok != test.wantOK {
				t.Fatalf("parseCGATT(%q) ok = %t, want %t", test.resp, ok, test.wantOK)
			}
			if ok && got != test.want {
				t.Fatalf("parseCGATT(%q) = %t, want %t", test.resp, got, test.want)
			}
		})
	}
}

// TestQueryPSAttachedWithoutATPort：AT 通道不可用时必须返回错误，
// 调用方（at_backend.GetServingSystem）据此保持零值，而不是误报已附着。
func TestQueryPSAttachedWithoutATPort(t *testing.T) {
	m, err := New(config.DeviceConfig{ID: "dev-ps-attach", DeviceBackend: "qmi"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := m.QueryPSAttached(); err == nil {
		t.Fatal("QueryPSAttached() on a manager without AT port should return an error")
	}
}
