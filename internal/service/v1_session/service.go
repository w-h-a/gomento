package v1session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	"github.com/w-h-a/gomento/internal/client/filer"
	"github.com/w-h-a/gomento/internal/client/persister"
	"github.com/w-h-a/gomento/internal/service"
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

func (s *V1Service) FinishSession(ctx context.Context, sessionId uuid.UUID) error {
	payload := v1.Payload{
		SessionId:       sessionId,
		TaskName:        "Distill Session",
		TaskDescription: fmt.Sprintf("Distilling session %s", sessionId),
		TaskStatus:      v1.TaskStatusPending,
	}
	task := &v1.Task{
		Id:        uuid.New(),
		Type:      v1.TaskTypeDistill,
		Payload:   payload,
		CreatedAt: time.Now(),
	}
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
