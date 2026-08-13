package main

import (
	"testing"

	"github.com/WongLoki/dj-4g-hub/pkg/smscodec"
)

func TestUSBLongSMSReassembly(t *testing.T) {
	r := smscodec.NewReassembler()
	second := smscodec.ConcatInfo{IsConcat: true, Ref: 42, RefBits: 8, Total: 2, Seq: 2}
	first := smscodec.ConcatInfo{IsConcat: true, Ref: 42, RefBits: 8, Total: 2, Seq: 1}

	if complete, content := r.Add("o2 Info", second, ", BIC: HYVEDEMMXXX."); complete || content != "" {
		t.Fatalf("second fragment completed early: complete=%v content=%q", complete, content)
	}
	complete, content := r.Add("o2 Info", first, "Zum Aufladen Ihres Prepaid Guthabens")
	if !complete {
		t.Fatal("two-part SMS was not completed")
	}
	if want := "Zum Aufladen Ihres Prepaid Guthabens, BIC: HYVEDEMMXXX."; content != want {
		t.Fatalf("reassembled content = %q, want %q", content, want)
	}
}
