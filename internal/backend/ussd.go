package backend

import (
	"context"
	"time"

	"github.com/WongLoki/DJ4Hub/internal/modem"
)

type USSDResult struct {
	Status    int    `json:"status"`
	Text      string `json:"text"`
	RawText   string `json:"raw_text"`
	DCS       int    `json:"dcs"`
	SessionID string `json:"session_id,omitempty"`
}

type USSDProvider interface {
	ExecuteUSSD(ctx context.Context, command string, timeout time.Duration) (*USSDResult, error)
	CancelUSSD(ctx context.Context) error
}

type USSDContinueProvider interface {
	ContinueUSSD(ctx context.Context, input string, timeout time.Duration) (*USSDResult, error)
}

func modemUSSDResult(in *modem.USSDResult) *USSDResult {
	if in == nil {
		return nil
	}
	return &USSDResult{
		Status:  in.Status,
		Text:    in.Text,
		RawText: in.RawText,
		DCS:     in.DCS,
	}
}
