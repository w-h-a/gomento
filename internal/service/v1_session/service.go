package v1session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
	"github.com/w-h-a/gomento/internal/util"
)

const (
	defaultMessagesLimit = 20

	assetPublicUrlExpire = 24 * time.Hour
)

type V1Service struct {
	*service.Service
	persister  persister.V1Persister
	dispatcher dispatcher.V1Dispatcher
	filer      filer.V1Filer
	qname      string
}

func (s *V1Service) Create(ctx context.Context, projectId uuid.UUID, spaceId *uuid.UUID) (*v1.Session, error) {
	p := &v1.Session{
		Id:        uuid.New(),
		ProjectId: projectId,
		SpaceId:   spaceId,
	}
	if err := s.persister.CreateSession(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *V1Service) ConnectToSpace(ctx context.Context, sessionId uuid.UUID, spaceId uuid.UUID) error {
	sess, err := s.persister.GetSession(ctx, sessionId)
	if err != nil {
		return err
	}

	if sess == nil {
		return ErrSessionNotFound
	}

	space, err := s.persister.GetSpace(ctx, spaceId)
	if err != nil {
		return err
	}
	if space == nil {
		return ErrSpaceNotFound
	}

	sess.SpaceId = &spaceId

	return s.persister.UpdateSession(ctx, sess)
}

func (s *V1Service) AddMessage(ctx context.Context, in SendMessageInput) (*v1.Message, error) {
	assets := map[int]*v1.Asset{}
	finalParts := []v1.Part{}

	for _, pIn := range in.Parts {
		domainPart := v1.Part{
			Type: pIn.Type,
			Text: pIn.Text,
			Meta: pIn.Meta,
		}

		if domainPart.Meta == nil {
			domainPart.Meta = map[string]any{}
		}

		if pIn.Type == "text" || pIn.Type == "tool-call" || pIn.Type == "tool-result" {
			finalParts = append(finalParts, domainPart)
			continue
		}

		if len(pIn.FileField) == 0 {
			return nil, fmt.Errorf("part type %s missing file_field", pIn.Type)
		}

		fh, ok := in.Files[pIn.FileField]
		if !ok {
			return nil, fmt.Errorf("file %s not found", pIn.FileField)
		}

		asset, err := s.filer.Upload(ctx, fh)
		if err != nil {
			return nil, fmt.Errorf("upload failed: %w", err)
		}

		asset.Id = uuid.New()

		domainPart.Meta["filename"] = fh.Filename

		currentPartIdx := len(finalParts)
		assets[currentPartIdx] = asset

		finalParts = append(finalParts, domainPart)
	}

	msg := &v1.Message{
		Id:        uuid.New(),
		SessionId: in.SessionId,
		Role:      in.Role,
		Parts:     finalParts,
	}

	if err := s.persister.CreateMessageWithAssets(ctx, msg, assets); err != nil {
		return nil, err
	}

	return msg, nil
}

func (s *V1Service) GetMessages(ctx context.Context, in GetMessagesInput) (*GetMessagesOutput, error) {
	var afterT time.Time
	var afterId uuid.UUID
	var err error

	if len(in.Cursor) > 0 {
		afterT, afterId, err = util.DecodeCursor(in.Cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to decode cursor: %w", err)
		}
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultMessagesLimit
	}

	if limit > 100 {
		limit = 100
	}

	msgs, err := s.persister.GetMessages(
		ctx,
		in.SessionId,
		persister.WithLimit(limit+1),
		persister.WithAfterCreatedAt(afterT),
		persister.WithAfterId(afterId),
	)
	if err != nil {
		return nil, err
	}

	out := &GetMessagesOutput{
		Items:   msgs,
		HasMore: false,
	}

	if len(msgs) > limit {
		out.HasMore = true
		out.Items = msgs[:limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor = util.EncodeCursor(last.CreatedAt, last.Id)
	}

	if !in.WithAssetPublicUrl {
		return out, nil
	}

	out.PublicUrls = map[uuid.UUID]PublicUrl{}

	var assetIds []uuid.UUID
	for _, m := range out.Items {
		for _, p := range m.Parts {
			if p.AssetId != nil {
				assetIds = append(assetIds, *p.AssetId)
			}
		}
	}

	assets, err := s.persister.GetAssets(ctx, assetIds)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch assets: %w", err)
	}

	for id, asset := range assets {
		url, err := s.filer.PresignGet(ctx, asset.Path, assetPublicUrlExpire)
		if err != nil {
			return nil, fmt.Errorf("presign failed for asset %s: %w", id, err)
		}
		out.PublicUrls[id] = PublicUrl{
			Url:      url,
			ExpireAt: time.Now().Add(assetPublicUrlExpire),
		}
	}

	return out, nil
}

func (s *V1Service) GetTasks(ctx context.Context, in GetTasksInput) (*GetTasksOutput, error) {
	tasks, err := s.persister.FetchCurrentTasks(ctx, in.SessionId, nil)
	if err != nil {
		return nil, err
	}

	out := &GetTasksOutput{
		Items: tasks,
	}

	return out, nil
}

func (s *V1Service) CheckpointSession(ctx context.Context, sessionId uuid.UUID) error {
	return s.dispatchJob(ctx, sessionId, v1.JobTypeExtract)
}

func (s *V1Service) FinishSession(ctx context.Context, sessionId uuid.UUID) error {
	return s.dispatchJob(ctx, sessionId, v1.JobTypeDistill)
}

func (s *V1Service) dispatchJob(ctx context.Context, sessionId uuid.UUID, scope string) error {
	payload := v1.JobPayload{
		SessionId: sessionId,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	job := &v1.Job{
		Id:      uuid.New(),
		Type:    scope,
		Payload: data,
		Status:  v1.JobStatusPending,
	}

	if err := s.persister.CreateJob(ctx, job); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	return s.dispatcher.Publish(ctx, job, dispatcher.PublishWithQueue(s.qname))
}

func NewV1Service(
	p persister.V1Persister,
	d dispatcher.V1Dispatcher,
	f filer.V1Filer,
	qname string,
) *V1Service {
	s := service.New()
	return &V1Service{
		Service:    s,
		persister:  p,
		dispatcher: d,
		filer:      f,
		qname:      qname,
	}
}
