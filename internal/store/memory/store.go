package memory

import (
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/store"
)

func NewRepositories() *store.Repositories {
	return &store.Repositories{
		Surfaces:  NewSurfaceRepo(),
		Agents:    NewAgentRepo(),
		Profiles:  NewProfileRepo(),
		Grants:    NewGrantRepo(),
		Envelopes: NewEnvelopeRepo(),
		Audit:     audit.NewMemoryRepository(),
	}
}
