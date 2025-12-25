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

func (s *V1Service) Create(ctx context.Context, projectId uuid.UUID, spaceId uuid.UUID) (*v1.Session, error) {
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

func (s *V1Service) AddMessage(ctx context.Context, in SendMessageInput) (*v1.Message, error) {
	assets := map[int]*v1.Asset{}
	finalParts := []v1.Part{}

	for _, pIn := range in.Parts {
		domainPart := v1.Part{
			Type: pIn.Type,
			Text: pIn.Text,
			Meta: pIn.Meta,
		}

		if pIn.Type == "text" {
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

func (s *V1Service) FinishSession(ctx context.Context, sessionId uuid.UUID) error {
	payload := v1.TaskPayload{
		Type:            v1.TaskTypeDistill,
		SessionId:       sessionId,
		TaskName:        "Distill Session",
		TaskDescription: fmt.Sprintf("Distilling session %s", sessionId),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := &v1.Task{
		Id:        uuid.New(),
		SessionId: sessionId,
		TaskOrder: 1,
		Data:      data,
		Status:    v1.TaskStatusPending,
	}

	// TODO: create task

	return s.dispatcher.Publish(ctx, task, dispatcher.PublishWithQueue(s.qname))
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
