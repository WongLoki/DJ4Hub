package backend

import "github.com/WongLoki/dj-4g-hub/pkg/mbim"

func (b *MBIMBackend) Capability() *mbim.Capabilities {
	return b.source.Capability()
}
