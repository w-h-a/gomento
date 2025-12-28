package v1mock

import (
	"context"
	"sync"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/interpreter"
)

type v1MockInterpreter struct {
	options        interpreter.Options
	skillRsp       *v1.Skill
	distillHistory []v1.Message
	actionRsp      []interpreter.TaskAction
	extractHistory []v1.Message
	mtx            sync.RWMutex
}

func (d *v1MockInterpreter) Distill(ctx context.Context, history []v1.Message) (*v1.Skill, error) {
	d.mtx.Lock()
	d.distillHistory = history
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

func (d *v1MockInterpreter) Extract(ctx context.Context, history []v1.Message, currentTasks []v1.Task) ([]interpreter.TaskAction, error) {
	d.mtx.Lock()
	d.extractHistory = history
	d.mtx.Unlock()

	if len(d.actionRsp) > 0 {
		return d.actionRsp, nil
	}

	return []interpreter.TaskAction{{Type: interpreter.TaskActionFinish}}, nil
}

func (d *v1MockInterpreter) DistillHistory() []v1.Message {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.distillHistory
}

func (d *v1MockInterpreter) ExtractHistory() []v1.Message {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.extractHistory
}

func NewV1Interpreter(opts ...interpreter.Option) *v1MockInterpreter {
	options := interpreter.NewOptions(opts...)

	d := &v1MockInterpreter{
		options:        options,
		distillHistory: []v1.Message{},
		extractHistory: []v1.Message{},
		mtx:            sync.RWMutex{},
	}

	if rsp, ok := SkillRspFrom(options.Context); ok {
		d.skillRsp = rsp
	}

	if rsp, ok := ActionRspFrom(options.Context); ok {
		d.actionRsp = rsp
	}

	return d
}
