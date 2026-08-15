package modem

import "testing"

func TestParseCNUM(t *testing.T) {
	tests := []struct {
		name string
		resp string
		want string
	}{
		{
			name: "quoted tel field",
			resp: "\r\n+CNUM: \"My Number\",\"+8613800138000\",145\r\n\r\nOK\r\n",
			want: "+8613800138000",
		},
		{
			name: "quoted without plus",
			resp: "\r\n+CNUM: \"Own Number\",\"13800138000\",129\r\n\r\nOK\r\n",
			want: "13800138000",
		},
		{
			name: "multi line prefers first valid number",
			resp: "\r\n+CNUM: \"Own Number\",\"\",129\r\n+CNUM: \"Line 1\",\"+8613900139000\",145\r\n\r\nOK\r\n",
			want: "+8613900139000",
		},
		{
			name: "placeholder value ignored",
			resp: "\r\n+CNUM: \"Own Number\",\"FFFFFFFF\",129\r\n\r\nOK\r\n",
			want: "",
		},
		{
			name: "missing cnum line",
			resp: "\r\nOK\r\n",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCNUM(tt.resp)
			if got != tt.want {
				t.Fatalf("parseCNUM()=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestGetFullStatusIncludesPhoneNumber(t *testing.T) {
	manager := &Manager{msisdn: "+8613800138000"}

	status := manager.GetFullStatus()
	if status.PhoneNumber != "+8613800138000" {
		t.Fatalf("GetFullStatus().PhoneNumber=%q want %q", status.PhoneNumber, "+8613800138000")
	}
}
