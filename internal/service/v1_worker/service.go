package v1worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tmc/langchaingo/textsplitter"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/buffer"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/interpreter"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxFileBytes      = 10 * 1024 * 1024 // 10MB
	ingestionInterval = 500 * time.Millisecond
	ingestionBatchMax = 50
	extractTimeout    = 60 * time.Second
)

type V1Service struct {
	*service.Service
	persister   persister.V1Persister
	buffer      buffer.V1Buffer
	dispatcher  dispatcher.V1Dispatcher
	filer       filer.V1Filer
	interpreter interpreter.V1Interpreter
	embedder    embedder.Embedder
	tracer      trace.Tracer
}

func (s *V1Service) Subscribe(ctx context.Context, cb func(context.Context, *v1.Job) error, qname string) error {
	return s.dispatcher.Subscribe(ctx, cb, dispatcher.SubscribeWithQueue(qname))
}

func (s *V1Service) StartIngestion(ctx context.Context) {
	ticker := time.NewTicker(ingestionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processIngestionBatch(ctx)
		}
	}
}

func (s *V1Service) Close(ctx context.Context) error {
	return s.dispatcher.Close(ctx)
}

func (s *V1Service) ProcessJob(ctx context.Context, job *v1.Job) error {
	ctx, span := s.tracer.Start(ctx, "worker.ProcessJob")
	defer span.End()

	span.SetAttributes(
		attribute.String("job.id", job.Id.String()),
		attribute.String("job.type", job.Type),
	)

	if err := s.persister.AcquireJobLock(ctx, job.Id); err != nil {
		if errors.Is(err, persister.ErrJobLocked) {
			slog.WarnContext(ctx, "job locked", "job_id", job.Id)
			span.AddEvent("job_locked")
			return nil
		}
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		span.RecordError(err)
		return fmt.Errorf("failed to acquire job lock: %w", err)
	}

	switch job.Type {
	case v1.JobTypeDistill:
		var payload v1.SessionJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return fmt.Errorf("invalid job payload: %w", err)
		}
		if err := s.distill(ctx, payload.SessionId, payload.MessageWindow); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return err
		}
	case v1.JobTypeIngestFile:
		var payload v1.IngestFileJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return fmt.Errorf("invalid job payload: %w", err)
		}
		if err := s.ingestFile(ctx, payload.FileId); err != nil {
			s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
			span.RecordError(err)
			return err
		}
	default:
		s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusFailed)
		err := fmt.Errorf("unknown job type: %s", job.Type)
		span.RecordError(err)
		return err
	}

	slog.InfoContext(ctx, "job success", "job_id", job.Id)
	span.AddEvent("job_success")

	return s.persister.UpdateJobStatus(ctx, job.Id, v1.JobStatusSuccess)
}

func (s *V1Service) processIngestionBatch(ctx context.Context) {
	ctx, span := s.tracer.Start(ctx, "worker.processIngestionBatch")
	defer span.End()

	batch, remaining, err := s.buffer.PopBatch(ctx, ingestionBatchMax)
	if err != nil {
		span.RecordError(err)
		slog.ErrorContext(ctx, "failed to pop ingestion batch", "error", err)
		return
	}

	span.SetAttributes(
		attribute.Int("batch.size", len(batch)),
		attribute.Int("queue.remaining", remaining),
	)

	if len(batch) == 0 {
		return
	}

	affectedSessions := map[uuid.UUID]struct{}{}

	for _, bm := range batch {
		if err := s.persister.CreateMessageWithAssets(ctx, &bm.Message, bm.Assets); err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to persist buffered message", "message_id", bm.Message.Id, "error", err)
			continue
		}

		if err := s.embedMessage(ctx, bm.Message.Id); err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to embed message", "message_id", bm.Message.Id, "error", err)
		}

		affectedSessions[bm.Message.SessionId] = struct{}{}
	}

	for sessionId := range affectedSessions {
		go func() {
			extractCtx, cancel := context.WithTimeout(context.Background(), extractTimeout)
			defer cancel()
			if err := s.extract(extractCtx, sessionId, 0); err != nil {
				slog.ErrorContext(ctx, "failed to extract", "session_id", sessionId, "error", err)
			}
		}()
	}
}

func (s *V1Service) embedMessage(ctx context.Context, messageId uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "worker.embedMessage")
	defer span.End()

	msg, err := s.persister.GetMessage(ctx, messageId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if msg == nil {
		return service.ErrMessageNotFound
	}

	var sb strings.Builder
	for _, p := range msg.Parts {
		if len(p.Text) > 0 {
			sb.WriteString(p.Text + "\n")
		}
	}

	text := sb.String()
	if len(strings.TrimSpace(text)) == 0 {
		return nil
	}

	vec, err := s.embedder.Embed(ctx, text)
	if err != nil {
		span.RecordError(err)
		return err
	}

	return s.persister.UpdateMessageEmbedding(ctx, messageId, vec)
}

func (s *V1Service) extract(ctx context.Context, sessionId uuid.UUID, messageWindow int) error {
	ctx, span := s.tracer.Start(ctx, "worker.extract")
	defer span.End()

	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if sess == nil {
		return service.ErrSessionNotFound
	}

	finalWindow := messageWindow
	if finalWindow < 1 {
		finalWindow = 20
	}

	recentMsgs, err := s.persister.ListMessages(
		ctx,
		sessionId,
		persister.WithSort(persister.SortOrderDesc),
		persister.WithLimit(finalWindow),
	)
	if err != nil {
		span.RecordError(err)
		return err
	}

	msgs := make([]v1.Message, len(recentMsgs))
	for i, msg := range recentMsgs {
		msgs[len(recentMsgs)-1-i] = msg
	}

	var contextQuery string
	for _, msg := range msgs {
		contextQuery += msg.Role + ": " + s.getMessageContent(msg) + "\n"
	}

	if len(contextQuery) == 0 {
		slog.InfoContext(ctx, "session has no context, skipping extraction", "session_id", sessionId)
		span.AddEvent("extraction_skipped")
		return nil
	}

	queryVec, err := s.embedder.Embed(ctx, contextQuery)
	if err != nil {
		span.RecordError(err)
		return err
	}

	searchSpaceId := uuid.Nil
	if sess.SpaceId != nil {
		searchSpaceId = *sess.SpaceId
	}

	chunks, err := s.persister.SearchMatchingChunks(
		ctx,
		searchSpaceId,
		queryVec,
		persister.SearchWithLimit(5),
	)
	if err != nil {
		span.RecordError(err)
		return err
	}

	maxIterations := 3

	for i := range maxIterations {
		span.AddEvent("extract_iteration_start", trace.WithAttributes(
			attribute.Int("iteration", i+1),
		))

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			span.RecordError(err)
			return err
		}

		actions, err := s.interpreter.Extract(ctx, msgs, chunks, tasks)
		if err != nil {
			span.RecordError(err)
			return err
		}

		if len(actions) == 0 {
			break
		}

		for _, action := range actions {
			if action.Type == interpreter.TaskActionFinish {
				span.AddEvent("action_finish")
				return nil
			}
			if err := s.executeTaskAction(ctx, sessionId, action, msgs); err != nil {
				span.RecordError(err)
				return err
			}
		}
	}

	return nil
}

func (s *V1Service) executeTaskAction(ctx context.Context, sessionId uuid.UUID, action interpreter.TaskAction, msgs []v1.Message) error {
	ctx, span := s.tracer.Start(ctx, "worker.executeTaskAction")
	defer span.End()

	span.SetAttributes(attribute.String("action.type", action.Type))

	p := action.Payload

	switch action.Type {
	case interpreter.TaskActionInsert:
		orderFloat, ok := p["after_task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'after_task_order' in payload")
		}
		order := int(orderFloat)

		desc, ok := p["task_description"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_description' in payload")
		}

		data, _ := json.Marshal(map[string]string{"task_description": desc})

		span.AddEvent("saving_task")

		if _, err := s.persister.InsertTask(ctx, sessionId, order, data, "pending"); err != nil {
			return err
		}

		return nil
	case interpreter.TaskActionUpdate:
		orderFloat, ok := p["task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_order' in payload")
		}
		order := int(orderFloat)

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			return err
		}

		var targetId uuid.UUID
		var currentData []byte
		found := false

		for _, t := range tasks {
			if t.TaskOrder == order {
				targetId = t.Id
				currentData = t.Data
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("task order %d not found", order)
		}

		var status *string
		if s, ok := p["task_status"].(string); ok {
			status = &s
		}

		var bs []byte
		if desc, ok := p["task_description"].(string); ok {
			existing := map[string]any{}
			_ = json.Unmarshal(currentData, &existing)
			existing["task_description"] = desc
			bs, _ = json.Marshal(existing)
		}

		span.AddEvent("updating_task")

		if _, err := s.persister.UpdateTask(ctx, targetId, status, nil, bs); err != nil {
			return err
		}

		return nil
	case interpreter.TaskActionAppendTask:
		orderFloat, ok := p["task_order"].(float64)
		if !ok {
			return fmt.Errorf("invalid or missing 'task_order' in payload")
		}
		order := int(orderFloat)

		tasks, err := s.persister.FetchCurrentTasks(ctx, sessionId, nil)
		if err != nil {
			return err
		}

		var targetId uuid.UUID
		found := false

		for _, t := range tasks {
			if t.TaskOrder == order {
				targetId = t.Id
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("task order %d not found", order)
		}

		indices, ok := p["message_ids"].([]any)
		if !ok {
			return fmt.Errorf("invalid or missing 'message_ids' in payload")
		}

		var ids []uuid.UUID
		for _, idx := range indices {
			if f, ok := idx.(float64); ok {
				i := int(f)
				if i >= 0 && i < len(msgs) {
					ids = append(ids, msgs[i].Id)
				}
			}
		}

		span.AddEvent("appending_messages_to_task")

		return s.persister.AppendMessagesToTask(ctx, targetId, ids)
	case interpreter.TaskActionAppendThought:
		indices, ok := p["message_ids"].([]any)
		if !ok {
			return fmt.Errorf("invalid or missing 'message_ids' in payload")
		}

		var ids []uuid.UUID
		for _, idx := range indices {
			if f, ok := idx.(float64); ok {
				i := int(f)
				if i >= 0 && i < len(msgs) {
					ids = append(ids, msgs[i].Id)
				}
			}
		}

		span.AddEvent("appending_messages_to_thought")

		return s.persister.AppendMessagesToThought(ctx, sessionId, ids)
	default:
		return nil
	}
}

func (s *V1Service) distill(ctx context.Context, sessionId uuid.UUID, messageWindow int) error {
	ctx, span := s.tracer.Start(ctx, "worker.distill")
	defer span.End()

	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if sess == nil {
		return service.ErrSessionNotFound
	}

	if sess.SpaceId == nil {
		slog.InfoContext(ctx, "session has no space, skipping distillation", "session_id", sessionId)
		span.AddEvent("distillation_skipped")
		return nil
	}

	finalWindow := messageWindow
	if finalWindow < 1 {
		finalWindow = 100
	}

	recentMsgs, err := s.persister.ListMessages(
		ctx,
		sessionId,
		persister.WithSort(persister.SortOrderDesc),
		persister.WithLimit(finalWindow),
	)
	if err != nil {
		span.RecordError(err)
		return err
	}

	msgs := make([]v1.Message, len(recentMsgs))
	for i, msg := range recentMsgs {
		msgs[len(recentMsgs)-1-i] = msg
	}

	var contextQuery string
	for _, msg := range msgs {
		contextQuery += msg.Role + ": " + s.getMessageContent(msg) + "\n"
	}

	if len(contextQuery) == 0 {
		slog.InfoContext(ctx, "session has no context, skipping distillation", "session_id", sessionId)
		span.AddEvent("distillation_skipped")
		return nil
	}

	queryVec, err := s.embedder.Embed(ctx, contextQuery)
	if err != nil {
		span.RecordError(err)
		return err
	}

	chunks, err := s.persister.SearchMatchingChunks(
		ctx,
		*sess.SpaceId,
		queryVec,
		persister.SearchWithLimit(5),
	)
	if err != nil {
		span.RecordError(err)
		return err
	}

	skills, err := s.persister.FetchCurrentSkills(ctx, *sess.SpaceId)
	if err != nil {
		span.RecordError(err)
		return err
	}

	actions, err := s.interpreter.Distill(ctx, msgs, chunks, skills)
	if err != nil {
		span.RecordError(err)
		return err
	}

	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if action.Type == interpreter.SkillActionFinish {
			span.AddEvent("action_finish")
			return nil
		}
		if err := s.executeSkillAction(ctx, sess, action); err != nil {
			span.RecordError(err)
			return err
		}
	}

	return nil
}

func (s *V1Service) executeSkillAction(ctx context.Context, sess *v1.Session, action interpreter.SkillAction) error {
	ctx, span := s.tracer.Start(ctx, "worker.executeSkillAction")
	defer span.End()

	span.SetAttributes(attribute.String("action.type", action.Type))

	p := action.Payload

	switch action.Type {
	case interpreter.SkillActionInsert:
		trigger, ok := p["trigger"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'trigger' in payload")
		}

		sop, ok := p["sop"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'sop' in payload")
		}

		vec, err := s.embedder.Embed(ctx, trigger)
		if err != nil {
			span.RecordError(err)
			return err
		}

		skill := &v1.Skill{
			Id:        uuid.New(),
			SpaceId:   *sess.SpaceId,
			Trigger:   trigger,
			SOP:       sop,
			Embedding: vec,
		}

		span.AddEvent("saving_skill")

		return s.persister.SaveSkill(ctx, skill)
	case interpreter.SkillActionUpdate:
		idStr, ok := p["skill_id"].(string)
		if !ok {
			return fmt.Errorf("missing 'skill_id' in payload")
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return fmt.Errorf("invalid 'skill_id' in payload")
		}

		trigger, ok := p["trigger"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'trigger' in payload")
		}

		sop, ok := p["sop"].(string)
		if !ok {
			return fmt.Errorf("invalid or missing 'sop' in payload")
		}

		vec, err := s.embedder.Embed(ctx, trigger)
		if err != nil {
			span.RecordError(err)
			return err

		}

		span.AddEvent("updating_skill")

		return s.persister.UpdateSkill(ctx, id, trigger, sop, vec)
	default:
		return nil
	}
}

func (s *V1Service) ingestFile(ctx context.Context, fileId uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "worker.ingestFile")
	defer span.End()

	file, err := s.persister.GetFile(ctx, fileId)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if file == nil {
		return service.ErrFileNotFound
	}
	if file.Asset == nil {
		return service.ErrFileNotUploaded
	}

	rc, err := s.filer.Download(ctx, file.Asset.Path)
	if err != nil {
		return err
	}
	defer rc.Close()

	content, err := io.ReadAll(io.LimitReader(rc, maxFileBytes))
	if err != nil {
		span.RecordError(err)
		return err
	}

	text := string(content)
	if len(strings.TrimSpace(text)) == 0 {
		return nil
	}

	splitter := textsplitter.NewRecursiveCharacter()

	splitter.ChunkSize = 1000
	splitter.ChunkOverlap = 200

	chunks, err := splitter.SplitText(text)
	if err != nil {
		span.RecordError(err)
		return err
	}

	var fileChunks []v1.FileChunk

	for i, content := range chunks {
		chunkText := fmt.Sprintf("Filename: %s\nChunk %d:\n%s", file.Filename, i+1, content)

		vec, err := s.embedder.Embed(ctx, chunkText)
		if err != nil {
			span.RecordError(err)
			return err
		}

		fileChunks = append(fileChunks, v1.FileChunk{
			Id:         uuid.New(),
			FileId:     file.Id,
			ChunkIndex: i,
			Content:    content,
			Embedding:  vec,
		})
	}

	if err := s.persister.SaveFileChunks(ctx, file.Id, fileChunks); err != nil {
		span.RecordError(err)
		return err
	}

	// Legacy
	if len(fileChunks) > 0 {
		_ = s.persister.UpdateFileEmbedding(ctx, file.Id, fileChunks[0].Embedding)
	}

	return nil
}

func (s *V1Service) getMessageContent(m v1.Message) string {
	var sb strings.Builder
	for _, p := range m.Parts {
		if len(p.Text) > 0 {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func NewV1Service(
	p persister.V1Persister,
	b buffer.V1Buffer,
	d dispatcher.V1Dispatcher,
	f filer.V1Filer,
	i interpreter.V1Interpreter,
	e embedder.Embedder,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:     s,
		persister:   p,
		buffer:      b,
		dispatcher:  d,
		filer:       f,
		interpreter: i,
		embedder:    e,
		tracer:      otel.Tracer("github.com/w-h-a/gomento/internal/service/v1_worker"),
	}
}
