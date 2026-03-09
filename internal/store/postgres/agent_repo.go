package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/store/sqltx"
)

var ErrNilDB = errors.New("postgres db is nil")

type AgentRepo struct {
	db sqltx.DBTX
}

func NewAgentRepo(db sqltx.DBTX) (*AgentRepo, error) {
	if db == nil {
		return nil, ErrNilDB
	}

	return &AgentRepo{db: db}, nil
}

func (r *AgentRepo) GetByID(ctx context.Context, id string) (*agent.Agent, error) {
	const q = `
		SELECT
			id,
			name,
			type,
			owner,
			model_version,
			endpoint,
			operational_state,
			created_at,
			updated_at
		FROM agents
		WHERE id = $1
	`

	var a agent.Agent

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&a.ID,
		&a.Name,
		&a.Type,
		&a.Owner,
		&a.ModelVersion,
		&a.Endpoint,
		&a.OperationalState,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &a, nil
}

func (r *AgentRepo) Create(ctx context.Context, a *agent.Agent) error {
	const q = `
		INSERT INTO agents (
			id,
			name,
			type,
			owner,
			model_version,
			endpoint,
			operational_state,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		a.ID,
		a.Name,
		a.Type,
		a.Owner,
		a.ModelVersion,
		a.Endpoint,
		a.OperationalState,
		a.CreatedAt,
		a.UpdatedAt,
	)
	return err
}

func (r *AgentRepo) Update(ctx context.Context, a *agent.Agent) error {
	const q = `
		UPDATE agents
		SET
			name = $2,
			type = $3,
			owner = $4,
			model_version = $5,
			endpoint = $6,
			operational_state = $7,
			updated_at = $8
		WHERE id = $1
	`

	_, err := r.db.ExecContext(
		ctx,
		q,
		a.ID,
		a.Name,
		a.Type,
		a.Owner,
		a.ModelVersion,
		a.Endpoint,
		a.OperationalState,
		a.UpdatedAt,
	)
	return err
}

func (r *AgentRepo) List(ctx context.Context) ([]*agent.Agent, error) {
	const q = `
		SELECT
			id,
			name,
			type,
			owner,
			model_version,
			endpoint,
			operational_state,
			created_at,
			updated_at
		FROM agents
		ORDER BY id
	`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*agent.Agent

	for rows.Next() {
		var a agent.Agent

		if err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.Type,
			&a.Owner,
			&a.ModelVersion,
			&a.Endpoint,
			&a.OperationalState,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, err
		}

		agents = append(agents, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return agents, nil
}

var _ agent.AgentRepository = (*AgentRepo)(nil)
