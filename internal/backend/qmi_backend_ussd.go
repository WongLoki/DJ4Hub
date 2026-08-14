package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WongLoki/DJ4Hub/pkg/logger"
	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

// QMI VOICE uses QmiVoiceUssDataCodingScheme, not the AT+CUSD GSM DCS value.
const qmiUSSDDCSDefault = 0x01

type qmiUSSDSource interface {
	VOICEOriginateUSSD(ctx context.Context, req qmi.VoiceUSSDRequest) (*qmi.VoiceUSSDResponse, error)
	VOICECancelUSSD(ctx context.Context) error
	OnVoiceUSSD(func(*qmi.VoiceUSSDIndication)) error
	OnVoiceUSSDReleased(func()) error
	OnVoiceUSSDNoWaitResult(func(*qmi.VoiceUSSDNoWaitIndication)) error
}

type qmiUSSDNoWaitSource interface {
	VOICEOriginateUSSDNoWait(ctx context.Context, req qmi.VoiceUSSDRequest) error
}

type qmiUSSDSystemSelectionSource interface {
	NASGetSystemSelectionPreference(ctx context.Context) (*qmi.SystemSelectionPreference, error)
	NASSetSystemSelectionPreference(ctx context.Context, pref qmi.SystemSelectionPreference) error
}

type qmiUSSDVoiceConfigSource interface {
	VOICEGetConfig(ctx context.Context, query qmi.VoiceConfigQuery) (*qmi.VoiceConfig, error)
}

func (q *QMIBackend) ExecuteUSSD(ctx context.Context, command string, timeout time.Duration) (*USSDResult, error) {
	src, err := q.qmiUSSDSource()
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.initUSSDCallbacks(src)
	if q.ussdInitErr != nil {
		return nil, q.ussdInitErr
	}

	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("USSD command is empty")
	}

	q.ussdMu.Lock()
	defer q.ussdMu.Unlock()
	q.resetUSSDChannelsLocked()
	if q.ussdAwaitRelease {
		return nil, errors.New("上次 USSD 会话尚未释放，请稍后重试")
	}

	opCtx, opCancel := context.WithTimeout(ctx, timeout)
	defer opCancel()

	req := qmi.VoiceUSSDRequest{DCS: qmiUSSDDCSDefault, Data: []byte(command)}
	q.prepareQMIUSSDVoiceState(opCtx, src)
	if noWaitSrc, ok := any(src).(qmiUSSDNoWaitSource); ok {
		err := noWaitSrc.VOICEOriginateUSSDNoWait(opCtx, req)
		if err == nil {
			logger.Debug("QMI USSD NoWait 已发起", "control_path", q.controlPath)
			return q.waitQMIUSSDResult(opCtx, src)
		}
		if !qmiUSSDNoWaitUnsupported(err) {
			if qmiUSSDOperationDeadlineExceeded(opCtx, err) {
				q.ussdAwaitRelease = true
				_ = src.VOICECancelUSSD(context.Background())
				return nil, errors.New("USSD 响应网络超时（QMI 异步请求超时）")
			}
			return nil, fmt.Errorf("发送 QMI USSD 失败: %w", qmiUSSDDecorateQMIError(err))
		}
		logger.Debug("QMI USSD NoWait 不可用，回退同步 originate",
			"control_path", q.controlPath,
			"err", qmiUSSDDecorateQMIError(err),
		)
	}

	resp, err := src.VOICEOriginateUSSD(opCtx, req)
	if result, resultErr, ok := qmiUSSDResultFromResponse(resp); ok {
		if resultErr != nil {
			if err != nil {
				return nil, fmt.Errorf("发送 QMI USSD 失败: %w; %v", qmiUSSDDecorateQMIError(err), resultErr)
			}
			return nil, resultErr
		}
		return result, nil
	}
	if err != nil {
		if qmiUSSDOperationDeadlineExceeded(opCtx, err) {
			q.ussdAwaitRelease = true
			_ = src.VOICECancelUSSD(context.Background())
			return nil, errors.New("USSD 响应网络超时（QMI 请求超时）")
		}
		return nil, fmt.Errorf("发送 QMI USSD 失败: %w", qmiUSSDDecorateQMIError(err))
	}

	return q.waitQMIUSSDResult(opCtx, src)
}

func (q *QMIBackend) waitQMIUSSDResult(ctx context.Context, src qmiUSSDSource) (*USSDResult, error) {
	select {
	case result := <-q.ussdResult:
		return &result, nil
	case err := <-q.ussdErr:
		return nil, err
	case <-q.ussdRelease:
		q.ussdAwaitRelease = false
		return &USSDResult{Status: 2, Text: "", RawText: "", DCS: int(qmiUSSDDCSDefault)}, nil
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			q.ussdAwaitRelease = true
			_ = src.VOICECancelUSSD(context.Background())
			return nil, ctx.Err()
		}
		q.ussdAwaitRelease = true
		_ = src.VOICECancelUSSD(context.Background())
		return nil, errors.New("USSD 响应网络超时（无回调）")
	}
}

func (q *QMIBackend) CancelUSSD(ctx context.Context) error {
	src, err := q.qmiUSSDSource()
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return src.VOICECancelUSSD(ctx)
}

func (q *QMIBackend) qmiUSSDSource() (qmiUSSDSource, error) {
	src, ok := q.source.(qmiUSSDSource)
	if !ok {
		return nil, fmt.Errorf("qmi source does not expose VOICE USSD")
	}
	return src, nil
}

func qmiUSSDOperationDeadlineExceeded(ctx context.Context, err error) bool {
	return err != nil && errors.Is(err, context.DeadlineExceeded) && ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func qmiUSSDNoWaitUnsupported(err error) bool {
	var qmiErr *qmi.QMIError
	if !errors.As(err, &qmiErr) {
		return false
	}
	switch qmiErr.ErrorCode {
	case qmi.QMIErrInvalidQmiCmd, qmi.QMIErrNotSupported, qmi.QMIErrOpDeviceUnsupported:
		return true
	default:
		return false
	}
}

func (q *QMIBackend) prepareQMIUSSDVoiceState(ctx context.Context, src qmiUSSDSource) {
	sysSrc, hasSystemSelection := any(src).(qmiUSSDSystemSelectionSource)
	var pref *qmi.SystemSelectionPreference
	if hasSystemSelection {
		current, err := sysSrc.NASGetSystemSelectionPreference(ctx)
		if err != nil {
			logger.Debug("QMI USSD 前置 NAS 系统选择读取失败", "control_path", q.controlPath, "err", err)
		} else {
			pref = current
		}
	}

	voiceDomainFromConfig, hasVoiceDomainFromConfig := q.queryQMIUSSDVoiceDomain(ctx, src)
	if hasSystemSelection && pref != nil {
		q.prepareQMIUSSDSystemSelection(ctx, sysSrc, pref, voiceDomainFromConfig, hasVoiceDomainFromConfig)
	}
}

func (q *QMIBackend) queryQMIUSSDVoiceDomain(ctx context.Context, src qmiUSSDSource) (uint32, bool) {
	voiceSrc, hasVoiceConfig := any(src).(qmiUSSDVoiceConfigSource)
	if !hasVoiceConfig {
		return 0, false
	}
	cfg, err := voiceSrc.VOICEGetConfig(ctx, qmi.VoiceConfigQuery{VoiceDomainPreference: true})
	if err != nil {
		logger.Debug("QMI USSD 前置 VOICE 配置读取失败", "control_path", q.controlPath, "err", err)
		return 0, false
	}
	if cfg == nil || !cfg.HasCurrentVoiceDomainPreference {
		logger.Debug("QMI USSD 前置 VOICE 配置未返回 voice domain", "control_path", q.controlPath)
		return 0, false
	}
	logger.Debug("QMI USSD 前置 VOICE 配置",
		"control_path", q.controlPath,
		"current_voice_domain", cfg.CurrentVoiceDomainPreference,
		"current_voice_domain_name", qmiUSSDVoiceDomainName(uint32(cfg.CurrentVoiceDomainPreference)),
	)
	return uint32(cfg.CurrentVoiceDomainPreference), true
}

func (q *QMIBackend) prepareQMIUSSDSystemSelection(ctx context.Context, src qmiUSSDSystemSelectionSource, current *qmi.SystemSelectionPreference, voiceDomainFromConfig uint32, hasVoiceDomainFromConfig bool) {
	if current == nil {
		logger.Debug("QMI USSD 前置 NAS 系统选择为空", "control_path", q.controlPath)
		return
	}

	fields := []any{"control_path", q.controlPath}
	if current.HasServiceDomainPreference {
		fields = append(fields,
			"service_domain", current.ServiceDomainPreference,
			"service_domain_name", qmiUSSDServiceDomainName(current.ServiceDomainPreference),
		)
	}
	if current.HasVoiceDomainPreference {
		fields = append(fields,
			"voice_domain", current.VoiceDomainPreference,
			"voice_domain_name", qmiUSSDVoiceDomainName(current.VoiceDomainPreference),
		)
	}
	if hasVoiceDomainFromConfig {
		fields = append(fields,
			"voice_domain_from_voice_config", voiceDomainFromConfig,
			"voice_domain_from_voice_config_name", qmiUSSDVoiceDomainName(voiceDomainFromConfig),
		)
	}
	logger.Debug("QMI USSD 前置 NAS 系统选择", fields...)

	next := qmi.SystemSelectionPreference{
		ChangeDuration:    qmi.NASChangeDurationPowerCycle,
		HasChangeDuration: true,
	}
	needsUpdate := false
	if current.HasServiceDomainPreference && current.ServiceDomainPreference == qmi.NASServiceDomainPreferencePSOnly {
		next.ServiceDomainPreference = qmi.NASServiceDomainPreferenceCSPS
		next.HasServiceDomainPreference = true
		needsUpdate = true
	}
	currentVoiceDomain := current.VoiceDomainPreference
	hasCurrentVoiceDomain := current.HasVoiceDomainPreference
	if !hasCurrentVoiceDomain && hasVoiceDomainFromConfig {
		currentVoiceDomain = voiceDomainFromConfig
		hasCurrentVoiceDomain = true
	}
	if hasCurrentVoiceDomain && (currentVoiceDomain == qmi.NASVoiceDomainPreferencePSOnly || currentVoiceDomain == qmi.NASVoiceDomainPreferencePSPreferred) {
		next.VoiceDomainPreference = qmi.NASVoiceDomainPreferenceCSPreferred
		next.HasVoiceDomainPreference = true
		needsUpdate = true
	}
	if !needsUpdate {
		return
	}
	if err := src.NASSetSystemSelectionPreference(ctx, next); err != nil {
		fields := []any{
			"control_path", q.controlPath,
			"err", err,
		}
		if next.HasServiceDomainPreference {
			fields = append(fields,
				"set_service_domain", next.ServiceDomainPreference,
				"set_service_domain_name", qmiUSSDServiceDomainName(next.ServiceDomainPreference),
			)
		}
		if next.HasVoiceDomainPreference {
			fields = append(fields,
				"set_voice_domain", next.VoiceDomainPreference,
				"set_voice_domain_name", qmiUSSDVoiceDomainName(next.VoiceDomainPreference),
			)
		}
		logger.Warn("QMI USSD 前置 CS/Voice 域准备失败，继续尝试发起 USSD",
			fields...,
		)
		return
	}
	fields = []any{
		"control_path", q.controlPath,
	}
	if next.HasServiceDomainPreference {
		fields = append(fields,
			"set_service_domain", next.ServiceDomainPreference,
			"set_service_domain_name", qmiUSSDServiceDomainName(next.ServiceDomainPreference),
		)
	}
	if next.HasVoiceDomainPreference {
		fields = append(fields,
			"set_voice_domain", next.VoiceDomainPreference,
			"set_voice_domain_name", qmiUSSDVoiceDomainName(next.VoiceDomainPreference),
		)
	}
	logger.Info("QMI USSD 前置 CS/Voice 域已准备", fields...)
}

func (q *QMIBackend) initUSSDCallbacks(src qmiUSSDSource) {
	q.ussdOnce.Do(func() {
		q.ussdResult = make(chan USSDResult, 1)
		q.ussdErr = make(chan error, 1)
		q.ussdRelease = make(chan struct{}, 1)
		if err := src.OnVoiceUSSD(func(info *qmi.VoiceUSSDIndication) {
			if result, ok := qmiUSSDResultFromIndication(info); ok {
				q.deliverUSSDResult(result)
			}
		}); err != nil {
			q.ussdInitErr = err
			return
		}
		if err := src.OnVoiceUSSDNoWaitResult(func(info *qmi.VoiceUSSDNoWaitIndication) {
			result, err, ok := qmiUSSDResultFromNoWait(info)
			if !ok {
				return
			}
			if err != nil {
				q.deliverUSSDError(err)
			} else {
				q.deliverUSSDResult(result)
			}
		}); err != nil {
			q.ussdInitErr = err
			return
		}
		if err := src.OnVoiceUSSDReleased(func() {
			select {
			case q.ussdRelease <- struct{}{}:
			default:
			}
		}); err != nil {
			q.ussdInitErr = err
			return
		}
	})
}

func (q *QMIBackend) resetUSSDChannelsLocked() {
	q.drainUSSDResultsLocked()
	q.drainUSSDErrorsLocked()
	if q.drainUSSDReleasesLocked() {
		q.ussdAwaitRelease = false
		q.drainUSSDResultsLocked()
		q.drainUSSDErrorsLocked()
	}
}

func (q *QMIBackend) drainUSSDResultsLocked() {
	for {
		select {
		case <-q.ussdResult:
		default:
			return
		}
	}
}

func (q *QMIBackend) drainUSSDErrorsLocked() {
	for {
		select {
		case <-q.ussdErr:
		default:
			return
		}
	}
}

func (q *QMIBackend) drainUSSDReleasesLocked() bool {
	drained := false
	for {
		select {
		case <-q.ussdRelease:
			drained = true
		default:
			return drained
		}
	}
}

func (q *QMIBackend) deliverUSSDResult(result USSDResult) {
	select {
	case q.ussdResult <- result:
	default:
	}
}

func (q *QMIBackend) deliverUSSDError(err error) {
	if err == nil {
		return
	}
	select {
	case q.ussdErr <- err:
	default:
	}
}

func qmiUSSDResultFromResponse(resp *qmi.VoiceUSSDResponse) (*USSDResult, error, bool) {
	if resp == nil {
		return nil, nil, false
	}
	if resp.HasFailureCause {
		return nil, fmt.Errorf("QMI USSD 失败: %s", qmiUSSDFailureCauseDetail(resp.FailureCause)), true
	}
	if resp.USSData != nil {
		result := qmiUSSDPayloadResult(0, resp.USSData)
		return &result, nil, true
	}
	if resp.Alpha != nil {
		result := qmiUSSDPayloadResult(0, resp.Alpha)
		return &result, nil, true
	}
	return nil, nil, false
}

func qmiUSSDResultFromIndication(info *qmi.VoiceUSSDIndication) (USSDResult, bool) {
	if info == nil || info.USSData == nil {
		return USSDResult{}, false
	}
	return qmiUSSDPayloadResult(0, info.USSData), true
}

func qmiUSSDResultFromNoWait(info *qmi.VoiceUSSDNoWaitIndication) (USSDResult, error, bool) {
	if info == nil {
		return USSDResult{}, nil, false
	}
	if info.HasErrorCode {
		return USSDResult{}, fmt.Errorf("QMI USSD 失败: error_code=0x%04X", info.ErrorCode), true
	}
	if info.HasFailureCause {
		return USSDResult{}, fmt.Errorf("QMI USSD 失败: %s", qmiUSSDFailureCauseDetail(info.FailureCause)), true
	}
	if info.USSData != nil {
		return qmiUSSDPayloadResult(0, info.USSData), nil, true
	}
	if info.Alpha != nil {
		return qmiUSSDPayloadResult(0, info.Alpha), nil, true
	}
	return USSDResult{}, nil, false
}

func qmiUSSDPayloadResult(status int, payload *qmi.VoiceUSSDPayload) USSDResult {
	text := strings.TrimSpace(payload.Text)
	raw := ""
	if len(payload.Data) > 0 {
		raw = string(payload.Data)
	}
	if raw == "" {
		raw = payload.Text
	}
	if text == "" {
		text = strings.TrimSpace(raw)
	}
	return USSDResult{Status: status, Text: text, RawText: raw, DCS: int(payload.DCS)}
}

func qmiUSSDDecorateQMIError(err error) error {
	if err == nil {
		return nil
	}
	var qmiErr *qmi.QMIError
	if errors.As(err, &qmiErr) {
		if name := qmiUSSDProtocolErrorName(qmiErr.ErrorCode); name != "" {
			return fmt.Errorf("%w (%s)", err, name)
		}
	}
	return err
}

func qmiUSSDProtocolErrorName(code uint16) string {
	switch code {
	case 0x005c:
		return "SUPS_FAILURE_CASE"
	case qmi.QMIErrInvalidArg:
		return "INVALID_ARG"
	case qmi.QMIErrNotSupported:
		return "NOT_SUPPORTED"
	case qmi.QMIErrOpDeviceUnsupported:
		return "OP_DEVICE_UNSUPPORTED"
	default:
		return ""
	}
}

func qmiUSSDFailureCauseDetail(cause uint16) string {
	if name := qmiUSSDFailureCauseName(cause); name != "" {
		return fmt.Sprintf("failure_cause=0x%04X (%s)", cause, name)
	}
	return fmt.Sprintf("failure_cause=0x%04X", cause)
}

func qmiUSSDFailureCauseName(cause uint16) string {
	switch cause {
	case 110:
		return "SUPSUnknownSubscriber"
	case 111:
		return "SUPSIllegalSubscriber"
	case 112:
		return "SUPSBearerServiceNotProvisioned"
	case 113:
		return "SUPSTeleserviceNotProvisioned"
	case 114:
		return "SUPSIllegalEquipment"
	case 115:
		return "SUPSCallBarred"
	case 116:
		return "SUPSIllegalSSOperation"
	case 117:
		return "SUPSSSErrorStatus"
	case 118:
		return "SUPSSSNotAvailable"
	case 119:
		return "SUPSSSSubscriptionViolation"
	case 120:
		return "SUPSSSIncompatibility"
	case 121:
		return "SUPSFacilityNotSupported"
	case 122:
		return "SUPSAbsentSubscriber"
	case 123:
		return "SUPSShortTermDenial"
	case 124:
		return "SUPSLongTermDenial"
	case 125:
		return "SUPSSystemFailure"
	case 126:
		return "SUPSDataMissing"
	case 127:
		return "SUPSUnexpectedDataValue"
	case 128:
		return "SUPSPasswordRegistrationFailure"
	case 129:
		return "SUPSNegativePasswordCheck"
	case 130:
		return "SUPSPasswordAttemptsViolation"
	case 131:
		return "SUPSPositionMethodFailure"
	case 132:
		return "SUPSUnknownAlphabet"
	case 133:
		return "SUPSUSSDBusy"
	case 134:
		return "SUPSRejectedByUser"
	case 135:
		return "SUPSRejectedByNetwork"
	case 136:
		return "SUPSDeflectionToServedSubscriber"
	case 137:
		return "SUPSSpecialServiceCode"
	case 138:
		return "SUPSInvalidDeflectedToNumber"
	case 139:
		return "SUPSMultipartyParticipantsExceeded"
	case 140:
		return "SUPSResourcesNotAvailable"
	case 218:
		return "VOICEWrongState/MMGMMWrongState"
	default:
		return ""
	}
}

func qmiUSSDServiceDomainName(domain uint32) string {
	switch domain {
	case qmi.NASServiceDomainPreferenceCSOnly:
		return "CSOnly"
	case qmi.NASServiceDomainPreferencePSOnly:
		return "PSOnly"
	case qmi.NASServiceDomainPreferenceCSPS:
		return "CSPS"
	case qmi.NASServiceDomainPreferencePSAttach:
		return "PSAttach"
	case qmi.NASServiceDomainPreferencePSDetach:
		return "PSDetach"
	default:
		return ""
	}
}

func qmiUSSDVoiceDomainName(domain uint32) string {
	switch domain {
	case qmi.NASVoiceDomainPreferenceCSOnly:
		return "CSOnly"
	case qmi.NASVoiceDomainPreferencePSOnly:
		return "PSOnly"
	case qmi.NASVoiceDomainPreferenceCSPreferred:
		return "CSPreferred"
	case qmi.NASVoiceDomainPreferencePSPreferred:
		return "PSPreferred"
	default:
		return ""
	}
}
