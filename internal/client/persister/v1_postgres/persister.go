package v1postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
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

func (p *v1PGPersister) CreateJob(ctx context.Context, job *v1.Job) error {
	query := `INSERT INTO jobs (id, type, payload, status) VALUES ($1, $2, $3, $4) RETURNING created_at, updated_at;`
	return p.conn.QueryRowContext(ctx, query, job.Id, job.Type, job.Payload, job.Status).Scan(&job.CreatedAt, &job.UpdatedAt)
}

func (p *v1PGPersister) AcquireJobLock(ctx context.Context, jobId uuid.UUID) error {
	query := `
		UPDATE jobs 
		SET status = 'running', updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`
	result, err := p.conn.ExecContext(ctx, query, jobId)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return persister.ErrJobLocked
	}

	return nil
}

func (p *v1PGPersister) UpdateJobStatus(ctx context.Context, jobId uuid.UUID, status string) error {
	query := `UPDATE jobs SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := p.conn.ExecContext(ctx, query, status, jobId)
	return err
}

func (p *v1PGPersister) CreateProject(ctx context.Context, proj *v1.Project) error {
	query := `INSERT INTO projects (id, name) VALUES ($1, $2) RETURNING created_at;`
	return p.conn.QueryRowContext(ctx, query, proj.Id, proj.Name).Scan(&proj.CreatedAt)
}

func (p *v1PGPersister) CreateSpace(ctx context.Context, space *v1.Space) error {
	query := `INSERT INTO spaces (id, project_id, name) VALUES ($1, $2, $3) RETURNING created_at;`
	return p.conn.QueryRowContext(ctx, query, space.Id, space.ProjectId, space.Name).Scan(&space.CreatedAt)
}

func (p *v1PGPersister) GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error) {
	query := `SELECT id, project_id, name, created_at FROM spaces WHERE id = $1`
	var s v1.Space

	err := p.conn.QueryRowContext(ctx, query, id).Scan(&s.Id, &s.ProjectId, &s.Name, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &s, nil
}

func (p *v1PGPersister) SaveSkill(ctx context.Context, skill *v1.Skill) error {
	query := `INSERT INTO skills (id, space_id, trigger, sop, embedding) VALUES ($1, $2, $3, $4, $5)`
	_, err := p.conn.ExecContext(ctx, query, skill.Id, skill.SpaceId, skill.Trigger, skill.SOP, pgvector.NewVector(skill.Embedding))
	return err
}

func (p *v1PGPersister) CreateSession(ctx context.Context, sess *v1.Session) error {
	query := `INSERT INTO sessions (id, project_id, space_id) VALUES ($1, $2, $3) RETURNING created_at;`
	return p.conn.QueryRowContext(ctx, query, sess.Id, sess.ProjectId, sess.SpaceId).Scan(&sess.CreatedAt)
}

func (p *v1PGPersister) GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error) {
	query := `SELECT id, project_id, space_id, created_at FROM sessions WHERE id = $1;`
	var sess v1.Session
	var spaceId uuid.NullUUID

	err := p.conn.QueryRowContext(ctx, query, id).Scan(&sess.Id, &sess.ProjectId, &spaceId, &sess.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if spaceId.Valid {
		sess.SpaceId = &spaceId.UUID
	}

	return &sess, nil
}

func (p *v1PGPersister) UpdateSession(ctx context.Context, sess *v1.Session) error {
	query := `UPDATE sessions SET space_id = $1 WHERE id = $2`
	_, err := p.conn.ExecContext(ctx, query, sess.SpaceId, sess.Id)
	return err
}

func (p *v1PGPersister) FetchCurrentTasks(ctx context.Context, sessionId uuid.UUID, status *string) ([]v1.Task, error) {
	query := `
        SELECT id, session_id, task_order, data, status, is_thought, created_at, updated_at 
        FROM tasks 
        WHERE session_id = $1 AND is_thought = FALSE`

	args := []any{sessionId}

	if status != nil {
		query += " AND status = $2"
		args = append(args, *status)
	}

	query += " ORDER BY task_order ASC"

	rows, err := p.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []v1.Task
	for rows.Next() {
		var t v1.Task
		if err := rows.Scan(&t.Id, &t.SessionId, &t.TaskOrder, &t.Data, &t.Status, &t.IsThought, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

func (p *v1PGPersister) InsertTask(ctx context.Context, sessionId uuid.UUID, afterOrder int, data []byte, status string) (*v1.Task, error) {
	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Shift existing tasks down to make room.
	// Step A: Negate
	negateQuery := `
        UPDATE tasks 
        SET task_order = -task_order 
        WHERE session_id = $1 
          AND task_order > $2 
          AND is_thought = FALSE`

	if _, err := tx.ExecContext(ctx, negateQuery, sessionId, afterOrder); err != nil {
		return nil, fmt.Errorf("failed to negate tasks for insert shift: %w", err)
	}

	// Step B: Shift
	shiftQuery := `
        UPDATE tasks 
        SET task_order = (-task_order) + 1 
        WHERE session_id = $1 
          AND task_order < (-$2::integer) 
          AND is_thought = FALSE`

	if _, err := tx.ExecContext(ctx, shiftQuery, sessionId, afterOrder); err != nil {
		return nil, fmt.Errorf("failed to shift tasks for insert: %w", err)
	}

	// 2. Insert the new task
	insertQuery := `
        INSERT INTO tasks (id, session_id, task_order, data, status, is_thought, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
        RETURNING id, session_id, task_order, data, status, is_thought, created_at, updated_at`

	var t v1.Task
	newID := uuid.New()

	err = tx.QueryRowContext(ctx, insertQuery,
		newID,
		sessionId,
		afterOrder+1,
		data,
		status,
		false,
	).Scan(&t.Id, &t.SessionId, &t.TaskOrder, &t.Data, &t.Status, &t.IsThought, &t.CreatedAt, &t.UpdatedAt)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &t, nil
}

func (p *v1PGPersister) UpdateTask(ctx context.Context, taskId uuid.UUID, status *string, order *int, data []byte) (*v1.Task, error) {
	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Lock Row & Get State
	var currentSessionId uuid.UUID
	var oldOrder int
	var isThought bool

	err = tx.QueryRowContext(ctx, "SELECT session_id, task_order, is_thought FROM tasks WHERE id = $1 FOR UPDATE", taskId).Scan(&currentSessionId, &oldOrder, &isThought)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, persister.ErrTaskNotFound
		}
		return nil, err
	}

	if isThought && order != nil {
		return nil, fmt.Errorf("cannot update task_order on a thought")
	}

	// 2. Handle Reordering
	if order != nil && !isThought && *order != oldOrder {
		newOrder := *order

		if newOrder > oldOrder {
			// Step A: Negate
			if _, err := tx.ExecContext(ctx, `
                UPDATE tasks SET task_order = -task_order 
                WHERE session_id = $1 AND task_order > $2 AND task_order <= $3 AND is_thought = FALSE`,
				currentSessionId, oldOrder, newOrder); err != nil {
				return nil, fmt.Errorf("failed to negate for move down: %w", err)
			}

			// Step B: Shift
			if _, err := tx.ExecContext(ctx, `
                UPDATE tasks SET task_order = (-task_order) - 1 
                WHERE session_id = $1 AND task_order < (-$2::integer) AND task_order >= (-$3::integer) AND is_thought = FALSE`,
				currentSessionId, oldOrder, newOrder); err != nil {
				return nil, fmt.Errorf("failed to shift for move down: %w", err)
			}

		} else {
			// Step A: Negate
			if _, err := tx.ExecContext(ctx, `
                UPDATE tasks SET task_order = -task_order 
                WHERE session_id = $1 AND task_order >= $2 AND task_order < $3 AND is_thought = FALSE`,
				currentSessionId, newOrder, oldOrder); err != nil {
				return nil, fmt.Errorf("failed to negate for move up: %w", err)
			}

			// Step B: Shift
			if _, err := tx.ExecContext(ctx, `
                UPDATE tasks SET task_order = (-task_order) + 1 
                WHERE session_id = $1 AND task_order <= (-$2::integer) AND task_order > (-$3::integer) AND is_thought = FALSE`,
				currentSessionId, newOrder, oldOrder); err != nil {
				return nil, fmt.Errorf("failed to shift for move up: %w", err)
			}
		}
	}

	// 3. Update Fields
	setParts := []string{"updated_at = NOW()"}
	args := []any{taskId}
	argIdx := 2

	if status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *status)
		argIdx++
	}

	if order != nil && !isThought {
		setParts = append(setParts, fmt.Sprintf("task_order = $%d", argIdx))
		args = append(args, *order)
		argIdx++
	}

	if data != nil {
		setParts = append(setParts, fmt.Sprintf("data = $%d", argIdx))
		args = append(args, data)
		argIdx++
	}

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = $1 RETURNING id, session_id, task_order, data, status, is_thought, created_at, updated_at", strings.Join(setParts, ", "))

	var t v1.Task
	err = tx.QueryRowContext(ctx, query, args...).Scan(&t.Id, &t.SessionId, &t.TaskOrder, &t.Data, &t.Status, &t.IsThought, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &t, nil
}

func (p *v1PGPersister) AppendMessagesToTask(ctx context.Context, taskId uuid.UUID, messageIds []uuid.UUID) error {
	if len(messageIds) == 0 {
		return nil
	}

	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1)", taskId).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return persister.ErrTaskNotFound
	}

	stringIds := make([]string, len(messageIds))
	for i, id := range messageIds {
		stringIds[i] = id.String()
	}

	if _, err := tx.ExecContext(ctx, `UPDATE messages SET task_id = $1 WHERE id = ANY($2::uuid[])`, taskId, pq.Array(stringIds)); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *v1PGPersister) AppendMessagesToThought(ctx context.Context, sessionId uuid.UUID, messageIds []uuid.UUID) error {
	if len(messageIds) == 0 {
		return nil
	}

	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var thoughtTaskId uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE session_id = $1 AND is_thought = true LIMIT 1`, sessionId).Scan(&thoughtTaskId)

	if err == sql.ErrNoRows {
		thoughtTaskId = uuid.New()
		_, err = tx.ExecContext(ctx, `
            INSERT INTO tasks (id, session_id, task_order, data, status, is_thought, created_at, updated_at) 
            VALUES ($1, $2, 0, '{}', 'pending', true, NOW(), NOW())`,
			thoughtTaskId, sessionId)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	stringIds := make([]string, len(messageIds))
	for i, id := range messageIds {
		stringIds[i] = id.String()
	}

	_, err = tx.ExecContext(ctx, `UPDATE messages SET task_id = $1 WHERE id = ANY($2::uuid[])`, thoughtTaskId, pq.Array(stringIds))
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (p *v1PGPersister) CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error {
	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// insert assets and ensure msg.Parts[idx] has correct AssetId
	for partIdx, a := range assets {
		row := tx.QueryRowContext(
			ctx,
			`INSERT INTO assets (id, container, path, etag, sha256, mime, size_bytes) 
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (container, path) DO UPDATE SET 
				etag=EXCLUDED.etag,
				sha256=EXCLUDED.sha256,
				mime=EXCLUDED.mime,
				size_bytes=EXCLUDED.size_bytes
			RETURNING id`,
			a.Id, a.Container, a.Path, a.ETag, a.SHA256, a.MIME, a.SizeBytes,
		)

		var finalAssetID uuid.UUID
		if err := row.Scan(&finalAssetID); err != nil {
			return err
		}

		if partIdx < len(msg.Parts) {
			msg.Parts[partIdx].AssetId = &finalAssetID
		}

		a.Id = finalAssetID
	}

	// insert message
	partsJson, err := json.Marshal(msg.Parts)
	if err != nil {
		return err
	}

	query := `WITH last_message AS (
		SELECT id FROM messages WHERE session_id = $1 ORDER BY created_at DESC LIMIT 1
	)
	INSERT INTO messages (id, session_id, parent_id, role, parts)
	VALUES ($2, $1, (SELECT id FROM last_message), $3, $4)
	RETURNING parent_id, created_at;`

	if err := tx.QueryRowContext(
		ctx,
		query,
		msg.SessionId,
		msg.Id,
		msg.Role,
		partsJson,
	).Scan(&msg.ParentId, &msg.CreatedAt); err != nil {
		return err
	}

	// link messages and assets
	for _, a := range assets {
		if _, err = tx.ExecContext(
			ctx,
			`INSERT INTO message_assets (message_id, asset_id) VALUES ($1, $2)`,
			msg.Id, a.Id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (p *v1PGPersister) ListMessages(ctx context.Context, sessionId uuid.UUID, opts ...persister.ListMessagesOption) ([]v1.Message, error) {
	options := persister.NewListMessagesOptions(opts...)

	query := `SELECT id, session_id, parent_id, role, parts, created_at FROM messages WHERE session_id = $1`
	args := []any{sessionId}
	argIdx := 2

	if !options.AfterCreatedAt.IsZero() && options.AfterId != uuid.Nil {
		if options.Sort == persister.SortOrderAsc {
			// return older first
			query += fmt.Sprintf(` AND (created_at > $%d OR (created_at = $%d AND id > $%d))`, argIdx, argIdx, argIdx+1)
		} else {
			// return newer first
			query += fmt.Sprintf(` AND (created_at < $%d OR (created_at = $%d AND id < $%d))`, argIdx, argIdx, argIdx+1)
		}
		args = append(args, options.AfterCreatedAt, options.AfterId)
		argIdx += 2
	}

	sortDir := "DESC"
	if options.Sort == persister.SortOrderAsc {
		sortDir = "ASC"
	}

	query += fmt.Sprintf(` ORDER BY created_at %s, id %s`, sortDir, sortDir)

	if options.Limit > 0 {
		query += fmt.Sprintf(` LIMIT $%d`, argIdx)
		args = append(args, options.Limit)
	}

	rows, err := p.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []v1.Message
	for rows.Next() {
		var m v1.Message
		var partsBytes []byte

		if err := rows.Scan(&m.Id, &m.SessionId, &m.ParentId, &m.Role, &partsBytes, &m.CreatedAt); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(partsBytes, &m.Parts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message parts: %w", err)
		}

		msgs = append(msgs, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return msgs, nil
}

func (p *v1PGPersister) GetAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*v1.Asset, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]*v1.Asset{}, nil
	}

	query := `SELECT id, container, path, mime, size_bytes FROM assets WHERE id = ANY($1)`

	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.String()
	}

	rows, err := p.conn.QueryContext(ctx, query, fmt.Sprintf("{%s}", strings.Join(idStrings, ",")))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]*v1.Asset)
	for rows.Next() {
		var a v1.Asset
		if err := rows.Scan(&a.Id, &a.Container, &a.Path, &a.MIME, &a.SizeBytes); err != nil {
			return nil, err
		}
		result[a.Id] = &a
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, rows.Err()
}

func (p *v1PGPersister) CreateArtifact(ctx context.Context, a *v1.Artifact) error {
	query := `INSERT INTO artifacts (id, project_id) VALUES ($1, $2) RETURNING created_at, updated_at;`
	return p.conn.QueryRowContext(ctx, query, a.Id, a.ProjectId).Scan(&a.CreatedAt, &a.UpdatedAt)
}

func (p *v1PGPersister) UpsertFileWithAsset(ctx context.Context, f *v1.File, a *v1.Asset) error {
	tx, err := p.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// upsert Asset
	assetQuery := `
		INSERT INTO assets (id, container, path, etag, sha256, mime, size_bytes) 
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (container, path) DO UPDATE SET 
			etag=EXCLUDED.etag,
			sha256=EXCLUDED.sha256,
			mime=EXCLUDED.mime,
			size_bytes=EXCLUDED.size_bytes
		RETURNING id, created_at`

	if err := tx.QueryRowContext(
		ctx, assetQuery,
		a.Id, a.Container, a.Path, a.ETag, a.SHA256, a.MIME, a.SizeBytes,
	).Scan(&a.Id, &a.CreatedAt); err != nil {
		return err
	}

	// link File to Asset
	f.AssetId = a.Id

	// upsert File Pointer
	fileQuery := `
		INSERT INTO files (id, artifact_id, asset_id, path, filename, meta)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (artifact_id, path, filename) 
		DO UPDATE SET 
			asset_id = EXCLUDED.asset_id, 
			meta = EXCLUDED.meta, 
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	if err := tx.QueryRowContext(ctx, fileQuery,
		f.Id, f.ArtifactId, f.AssetId, f.Path, f.Filename, f.Meta,
	).Scan(&f.Id, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *v1PGPersister) ListArtifacts(ctx context.Context, projectId uuid.UUID) ([]v1.Artifact, error) {
	query := `SELECT id, project_id, created_at, updated_at FROM artifacts WHERE project_id = $1`

	rows, err := p.conn.QueryContext(ctx, query, projectId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []v1.Artifact
	for rows.Next() {
		var a v1.Artifact
		if err := rows.Scan(&a.Id, &a.ProjectId, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return artifacts, nil
}

func (p *v1PGPersister) ListFiles(ctx context.Context, artifactId uuid.UUID, opts ...persister.ListFilesOption) ([]v1.File, error) {
	options := persister.NewListFilesOptions(opts...)

	query := `
		SELECT 
			f.id, f.artifact_id, f.asset_id, f.path, f.filename, f.meta, f.created_at, f.updated_at,
			a.id, a.container, a.path, a.mime, a.size_bytes
		FROM files f
		JOIN assets a ON f.asset_id = a.id
		WHERE f.artifact_id = $1`
	args := []any{artifactId}

	if len(options.PathPrefix) > 0 {
		query += " AND f.path LIKE $2"
		args = append(args, options.PathPrefix+"%")
	}

	query += ` ORDER BY f.path ASC, f.filename ASC`

	rows, err := p.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []v1.File
	for rows.Next() {
		var f v1.File
		f.Asset = &v1.Asset{}
		if err := rows.Scan(
			&f.Id, &f.ArtifactId, &f.AssetId, &f.Path, &f.Filename, &f.Meta, &f.CreatedAt, &f.UpdatedAt,
			&f.Asset.Id, &f.Asset.Container, &f.Asset.Path, &f.Asset.MIME, &f.Asset.SizeBytes,
		); err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}

func (p *v1PGPersister) GetFile(ctx context.Context, artifactId uuid.UUID, path string, filename string) (*v1.File, error) {
	query := `
		SELECT f.id, f.artifact_id, f.asset_id, f.path, f.filename, f.meta, f.created_at, f.updated_at,
		       a.id, a.container, a.path, a.mime, a.size_bytes
		FROM files f
		JOIN assets a ON f.asset_id = a.id
		WHERE f.artifact_id = $1 AND f.path = $2 AND f.filename = $3`

	var f v1.File
	var a v1.Asset

	err := p.conn.QueryRowContext(ctx, query, artifactId, path, filename).Scan(
		&f.Id, &f.ArtifactId, &f.AssetId, &f.Path, &f.Filename, &f.Meta, &f.CreatedAt, &f.UpdatedAt,
		&a.Id, &a.Container, &a.Path, &a.MIME, &a.SizeBytes,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	f.Asset = &a

	return &f, nil
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
