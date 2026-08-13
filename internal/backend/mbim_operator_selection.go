package backend

import (
	"context"

	"github.com/WongLoki/dj-4g-hub/pkg/mbim"
)

func (b *MBIMBackend) ScanOperators(ctx context.Context) ([]OperatorCandidate, error) {
	// MBIM VISIBLE_PROVIDERS is a single command/response scan. Unlike QMI NAS
	// incremental scans, it does not expose partial candidates, so the device SSE
	// path should report "running" until this method returns.
	providers, err := b.source.VisibleProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]OperatorCandidate, 0, len(providers))
	for _, provider := range providers {
		candidate := OperatorCandidate{
			PLMN:         provider.PLMN,
			OperatorName: provider.Name,
			Status:       mbimProviderStatus(provider.State),
			RATs:         mbimProviderRATs(provider.CellularClass),
		}
		if normalized, err := NormalizePLMNSelection(candidate.PLMN); err == nil {
			candidate.MCC = normalized.MCC
			candidate.MNC = normalized.MNC
			candidate.MNCLength = normalized.MNCLength
			candidate.IncludesPCSDigit = normalized.IncludesPCSDigit
		}
		out = append(out, candidate)
	}
	return out, nil
}

func (b *MBIMBackend) GetOperatorSelection(ctx context.Context) (OperatorSelection, error) {
	rs, err := b.source.GetRegisterState(ctx)
	if err != nil {
		return OperatorSelection{}, err
	}
	if rs.RegisterMode != mbim.RegisterModeManual {
		return OperatorSelection{Mode: OperatorSelectionAutomatic}, nil
	}
	sel, err := NormalizeManualOperatorSelection(rs.ProviderID, OperatorRATUnknown, nil)
	if err != nil {
		return OperatorSelection{}, err
	}
	sel.OperatorName = rs.ProviderName
	return sel, nil
}

func (b *MBIMBackend) SetOperatorSelection(ctx context.Context, req SetOperatorSelectionRequest) (OperatorSelection, error) {
	sel, err := NormalizeSetOperatorSelectionRequest(req)
	if err != nil {
		return OperatorSelection{}, err
	}
	if sel.Mode == OperatorSelectionAutomatic {
		if _, err := b.source.SetRegister(ctx, mbim.RegisterActionAutomatic, ""); err != nil {
			return OperatorSelection{}, err
		}
		return OperatorSelection{Mode: OperatorSelectionAutomatic}, nil
	}
	if _, err := b.source.SetRegister(ctx, mbim.RegisterActionManual, sel.PLMN); err != nil {
		return OperatorSelection{}, err
	}
	return sel, nil
}

func mbimProviderStatus(state uint32) string {
	switch {
	case state&(1<<4) != 0:
		return "current"
	case state&(1<<1) != 0:
		return "forbidden"
	case state&(1<<3) != 0:
		return "available"
	default:
		return "unknown"
	}
}

func mbimProviderRATs(class uint32) []OperatorAccessTechnology {
	if class&mbim.CellularClassGSM != 0 {
		return []OperatorAccessTechnology{OperatorRATGSM}
	}
	return nil
}
