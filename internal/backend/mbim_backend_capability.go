package backend

import "github.com/WongLoki/DJ4Hub/pkg/mbim"

func (b *MBIMBackend) Capability() *mbim.Capabilities {
	return b.source.Capability()
}
