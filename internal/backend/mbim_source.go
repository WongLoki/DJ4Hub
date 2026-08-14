package backend

import (
	"context"
	"time"

	"github.com/WongLoki/DJ4Hub/pkg/mbim"
	"github.com/iniwex5/quectel-qmi-go/pkg/manager"
)

// MBIMSource is the adapter surface MBIMBackend needs from internal/mbim.
type MBIMSource interface {
	ControlDevice() string

	DeviceCaps(ctx context.Context) (mbim.Caps, error)
	SubscriberReady(ctx context.Context) (mbim.SubscriberReady, error)
	RegisterState(ctx context.Context) (mbim.RegisterState, error)
	VisibleProviders(ctx context.Context) ([]mbim.Provider, error)
	HomeProvider(ctx context.Context) (mbim.Provider, error)
	GetRegisterState(ctx context.Context) (mbim.RegisterState, error)
	SetRegister(ctx context.Context, action uint32, plmn string) (mbim.RegisterState, error)
	SignalState(ctx context.Context) (mbim.SignalState, error)
	PacketService(ctx context.Context) (mbim.PacketService, error)
	SetPacketService(ctx context.Context, action mbim.PacketServiceAction) (mbim.PacketService, error)
	RadioState(ctx context.Context) (mbim.RadioState, error)
	SetRadioState(ctx context.Context, sw mbim.RadioSwitch) (mbim.RadioState, error)
	DeviceReset(ctx context.Context) error
	Snapshot() mbim.Snapshot
	Capability() *mbim.Capabilities

	SendSMS(ctx context.Context, pdu []byte) (uint32, error)
	ReadSMS(ctx context.Context, index uint32) (mbim.SMSRecord, error)
	ListSMS(ctx context.Context) ([]mbim.SMSRecord, error)
	DeleteSMS(ctx context.Context, index uint32) error
	DeleteAllSMS(ctx context.Context) error
	GetSMSC(ctx context.Context) (string, error)
	SetSMSC(ctx context.Context, smsc string) error

	ExecuteUSSD(ctx context.Context, command string, timeout time.Duration) (mbim.USSDResult, error)
	ContinueUSSD(ctx context.Context, input string, timeout time.Duration) (mbim.USSDResult, error)
	CancelUSSD(ctx context.Context) error

	OpenChannel(ctx context.Context, aid []byte) (uint32, error)
	CloseChannel(ctx context.Context, channel uint32) error
	TransmitAPDU(ctx context.Context, channel uint32, command []byte) ([]byte, error)
	ReadSIMEF(ctx context.Context, fileID uint16, readLen int) ([]byte, error)
	ReadSIMRecordEF(ctx context.Context, fileID uint16, recordNumber uint32) ([]byte, error)
	ResolveLogicalChannelAID(app string, fallback string) (string, string, error)
	GetUIMReadiness(ctx context.Context) (manager.UIMReadiness, error)
	UIMPowerOffSIM(ctx context.Context, slot uint8) error
	UIMPowerOnSIM(ctx context.Context, slot uint8) error

	CalculateAKA(ctx context.Context, rand, autn []byte) (res, ik, ck, auts []byte, err error)
}
