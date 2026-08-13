package backend

import (
	"context"
	"time"

	"github.com/WongLoki/dj-4g-hub/pkg/mbim"
)

func (b *MBIMBackend) ExecuteUSSD(ctx context.Context, command string, timeout time.Duration) (*USSDResult, error) {
	result, err := b.source.ExecuteUSSD(ctx, command, timeout)
	if err != nil {
		return nil, err
	}
	return mbimUSSDResult(result), nil
}

func (b *MBIMBackend) ContinueUSSD(ctx context.Context, input string, timeout time.Duration) (*USSDResult, error) {
	result, err := b.source.ContinueUSSD(ctx, input, timeout)
	if err != nil {
		return nil, err
	}
	return mbimUSSDResult(result), nil
}

func mbimUSSDResult(result mbim.USSDResult) *USSDResult {
	return &USSDResult{
		Status:  int(result.Response),
		Text:    result.Text,
		RawText: result.RawHex,
		DCS:     int(result.DCS),
	}
}

func (b *MBIMBackend) CancelUSSD(ctx context.Context) error {
	return b.source.CancelUSSD(ctx)
}
