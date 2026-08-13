package main

import (
	"testing"

	"github.com/WongLoki/dj-4g-hub/pkg/smscodec"
)

func TestATResponseCompleteRecognizesModemErrors(t *testing.T) {
	for _, response := range []string{
		"\r\n+CMS ERROR: 500\r\n",
		"\r\n+CME ERROR: 30\r\n",
		"\r\nERROR\r\n",
	} {
		if !atResponseComplete(response) {
			t.Fatalf("atResponseComplete(%q) = false", response)
		}
	}
}

func TestATResponseHasPrompt(t *testing.T) {
	if !atResponseHasPrompt("AT+CMGS=23\r\n> ") {
		t.Fatal("SMS prompt was not detected")
	}
	if atResponseHasPrompt("AT+CMGS=23\r\nOK\r\n") {
		t.Fatal("normal AT response was mistaken for a prompt")
	}
}

func TestSMSSubmitOptionsUsesUCS2ForChinese(t *testing.T) {
	if got := smsSubmitOptions("验证码 1234").Encoding; got != smscodec.SMSEncodingUCS2 {
		t.Fatalf("Chinese encoding = %q, want %q", got, smscodec.SMSEncodingUCS2)
	}
	if got := smsSubmitOptions("hello 123").Encoding; got != "" {
		t.Fatalf("ASCII encoding = %q, want auto", got)
	}
}
