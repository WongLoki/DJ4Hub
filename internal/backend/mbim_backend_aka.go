package backend

import (
	"context"
	"errors"
	"time"

	"github.com/WongLoki/dj-4g-hub/pkg/mbim"
)

// CalculateAKA computes AKA via the MBIM Auth service.
// Returns res, ik, ck, auts, err
func (b *MBIMBackend) CalculateAKA(ctx context.Context, rand16, autn16 []byte) (res, ik, ck, auts []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, ik, ck, auts, err = b.source.CalculateAKA(ctx, rand16, autn16)
	if err != nil && isAuthAKAUnsupported(err) {
		if caps := b.source.Capability(); caps != nil {
			caps.MarkAuthAKADead()
		}
	}
	return res, ik, ck, auts, err
}

func isAuthAKAUnsupported(err error) bool {
	var se *mbim.StatusError
	// Only MBIM_STATUS_NO_DEVICE_SUPPORT (9) means the feature is absent.
	// MBIM_STATUS_AUTH_SYNC_FAILURE (35) is a legitimate authentication result
	// (SQN mismatch) and must NOT disable the Auth service.
	return errors.As(err, &se) && se.Status == 9
}
