package apduarbiter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/WongLoki/dj-4g-hub/pkg/logger"
)

var ErrSIMAuthNotReady = errors.New("simauth not ready")

type SIMAuthReadyProbe func(context.Context) error

type simAuthReadyState struct {
	generation uint64
	ready      bool
	probing    bool
	waitC      chan struct{}
	lastErr    error
	reason     string
}

func (a *Arbiter) InvalidateSIMAuthReady(reason string) {
	if a == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}

	a.mu.Lock()
	a.simAuthReady.generation++
	a.simAuthReady.ready = false
	a.simAuthReady.lastErr = nil
	a.simAuthReady.reason = reason
	generation := a.simAuthReady.generation
	probing := a.simAuthReady.probing
	a.mu.Unlock()

	logger.Debug("SIMAuth readiness invalidated",
		"device", a.deviceID,
		"reason", reason,
		"generation", generation,
		"probing", probing)
}

func (a *Arbiter) WaitSIMAuthReady(ctx context.Context, probe SIMAuthReadyProbe) error {
	if a == nil {
		return fmt.Errorf("%w: arbiter nil", ErrSIMAuthNotReady)
	}
	if probe == nil {
		return fmt.Errorf("%w: probe nil", ErrSIMAuthNotReady)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: %v", ErrSIMAuthNotReady, err)
		}

		a.mu.Lock()
		if a.simAuthReady.ready {
			a.mu.Unlock()
			return nil
		}
		if a.simAuthReady.probing {
			waitC := a.simAuthReady.waitC
			if waitC == nil {
				waitC = make(chan struct{})
				a.simAuthReady.waitC = waitC
			}
			a.mu.Unlock()
			select {
			case <-ctx.Done():
				return fmt.Errorf("%w: %v", ErrSIMAuthNotReady, ctx.Err())
			case <-waitC:
				continue
			}
		}

		generation := a.simAuthReady.generation
		waitC := make(chan struct{})
		a.simAuthReady.probing = true
		a.simAuthReady.waitC = waitC
		a.mu.Unlock()

		err := probe(ctx)

		a.mu.Lock()
		stale := a.simAuthReady.generation != generation
		if !stale {
			a.simAuthReady.ready = err == nil
			a.simAuthReady.lastErr = err
		}
		if a.simAuthReady.waitC == waitC {
			close(waitC)
			a.simAuthReady.waitC = nil
		}
		a.simAuthReady.probing = false
		a.mu.Unlock()

		if err != nil {
			return fmt.Errorf("%w: %v", ErrSIMAuthNotReady, err)
		}
		if stale {
			continue
		}
		return nil
	}
}
