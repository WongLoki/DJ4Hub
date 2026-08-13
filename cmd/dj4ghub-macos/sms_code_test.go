package main

import "testing"

func TestExtractSMSCode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "Chinese", content: "【服务】您的验证码为 482913，5 分钟内有效。", want: "482913"},
		{name: "English", content: "Your verification code is 123456. Do not share it.", want: "123456"},
		{name: "Code before keyword", content: "登录动态码：778899", want: "778899"},
		{name: "No keyword", content: "您的号码 13800138000，本月余额 128.50 元。", want: ""},
		{name: "Too short", content: "验证码 123", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := extractSMSCode(test.content); got != test.want {
				t.Fatalf("extractSMSCode(%q) = %q, want %q", test.content, got, test.want)
			}
		})
	}
}
