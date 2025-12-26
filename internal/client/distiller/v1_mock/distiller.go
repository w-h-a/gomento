package v1mock

import (
	"context"
	"sync"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/distiller"
)

type v1MockDistiller struct {
	options  distiller.Options
	skillRsp *v1.Skill
	history  []v1.Message
	mtx      sync.RWMutex
}

func (d *v1MockDistiller) Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error) {
	d.mtx.Lock()
	d.history = history
	d.mtx.Unlock()

	if d.skillRsp != nil {
		return d.skillRsp, nil
	}

	embedding := make([]float32, 1536)
	embedding[0] = 0.01

	return &v1.Skill{
		Id:        uuid.New(),
		Trigger:   "how to restart redis",
		SOP:       "1. Check logs.\n2. Delete pod.",
		Embedding: embedding,
	}, nil
}

func (d *v1MockDistiller) History() []v1.Message {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.history
}

func NewV1Distiller(opts ...distiller.Option) *v1MockDistiller {
	options := distiller.NewOptions(opts...)

	d := &v1MockDistiller{
		options: options,
		history: []v1.Message{},
		mtx:     sync.RWMutex{},
	}

	if rsp, ok := SkillRspFrom(options.Context); ok {
		d.skillRsp = rsp
	}

	return d
}
