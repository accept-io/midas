package postgres

import (
	"database/sql"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/store"
)

func NewRepositories(db *sql.DB) (*store.Repositories, error) {
	surfaces, err := NewSurfaceRepo(db)
	if err != nil {
		return nil, err
	}

	agents, err := NewAgentRepo(db)
	if err != nil {
		return nil, err
	}

	profiles, err := NewProfileRepo(db)
	if err != nil {
		return nil, err
	}

	grants, err := NewGrantRepo(db)
	if err != nil {
		return nil, err
	}

	envelopes, err := NewEnvelopeRepo(db)
	if err != nil {
		return nil, err
	}

	return &store.Repositories{
		Surfaces:  surfaces,
		Agents:    agents,
		Profiles:  profiles,
		Grants:    grants,
		Envelopes: envelopes,
		Audit:     audit.NewPostgresRepository(db),
	}, nil
}
