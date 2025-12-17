package v1postgres

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/persister"
	"go.nhat.io/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var DRIVER string

func init() {
	driver, err := otelsql.Register(
		"postgres",
		otelsql.TraceQueryWithoutArgs(),
		otelsql.TraceRowsClose(),
		otelsql.TraceRowsAffected(),
		otelsql.WithSystem(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		detail := "failed to register v1 pg persister with otel"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	DRIVER = driver
}

type v1PGPersister struct {
	options persister.Options
	conn    *sql.DB
}

func (p *v1PGPersister) CreateProject(ctx context.Context, proj *v1.Project) error {
	query := `INSERT INTO projects (name) VALUES ($1);`
	_, err := p.conn.ExecContext(ctx, query, proj.Name)
	return err
}

func (p *v1PGPersister) CreateSpace(ctx context.Context, space *v1.Space) error {
	query := `INSERT INTO spaces (project_id, name) VALUES ($1, $2);`
	_, err := p.conn.ExecContext(ctx, query, space.ProjectId, space.Name)
	return err
}

func (p *v1PGPersister) CreateSession(ctx context.Context, sess *v1.Session) error {
	query := `INSERT INTO sessions (project_id, space_id) VALUES ($1, $2);`
	_, err := p.conn.ExecContext(ctx, query, sess.ProjectId, sess.SpaceId)
	return err
}

func (p *v1PGPersister) GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error) {
	query := `SELECT id, project_id, space_id, created_at FROM sessions WHERE id = $1;`
	var sess v1.Session
	err := p.conn.QueryRowContext(ctx, query, id).Scan(&sess.Id, &sess.ProjectId, &sess.SpaceId, &sess.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (p *v1PGPersister) AddMessage(ctx context.Context, msg *v1.Message) error {
	query := `INSERT INTO messages (session_id, role, content) VALUES ($1, $2, $3);`
	_, err := p.conn.ExecContext(ctx, query, msg.SessionId, msg.Role, msg.Content)
	return err
}

func (p *v1PGPersister) GetMessages(ctx context.Context, sessionId uuid.UUID) ([]v1.Message, error) {
	query := `SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = $1 ORDER BY created_at ASC;`
	rows, err := p.conn.QueryContext(ctx, query, sessionId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []v1.Message
	for rows.Next() {
		var m v1.Message
		if err := rows.Scan(&m.Id, &m.SessionId, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return msgs, nil
}

func (p *v1PGPersister) SaveSkill(ctx context.Context, skill *v1.Skill) error {
	query := `INSERT INTO skills (space_id, trigger, sop, embedding) VALUES ($1, $2, $3, $4)`
	_, err := p.conn.ExecContext(ctx, query, skill.SpaceId, skill.Trigger, skill.SOP, pgvector.NewVector(skill.Embedding))
	return err
}

func NewV1Persister(opts ...persister.Option) persister.V1Persister {
	options := persister.NewOptions(opts...)

	// TODO: validate options

	p := &v1PGPersister{
		options: options,
	}

	// postgres://user:password@host:port/db?sslmode=disable
	conn, err := sql.Open(DRIVER, p.options.Location)
	if err != nil {
		detail := "failed to connect with v1 pg persister"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	if err := conn.Ping(); err != nil {
		detail := "failed to ping with v1 pg persister"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	if err := otelsql.RecordStats(conn); err != nil {
		detail := "failed to initialize postgres instrumentation for v1 pg persister"
		slog.ErrorContext(context.Background(), detail, "error", err)
		panic(detail)
	}

	p.conn = conn

	return p
}
