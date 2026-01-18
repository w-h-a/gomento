package v1mock

import (
	"context"
	"sync"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/interpreter"
)

type v1MockInterpreter struct {
	options        interpreter.Options
	distillRsp     []interpreter.SkillAction
	extractRsp     []interpreter.TaskAction
	distillHistory []v1.Message
	distillSkills  []v1.Skill
	extractHistory []v1.Message
	extractFiles   []v1.File
	extractTasks   []v1.Task
	mtx            sync.RWMutex
}

func (d *v1MockInterpreter) Distill(ctx context.Context, history []v1.Message, messageWindow int, currentSkills []v1.Skill) ([]interpreter.SkillAction, error) {
	d.mtx.Lock()
	d.distillHistory = history
	d.distillSkills = currentSkills
	d.mtx.Unlock()

	if d.distillRsp != nil {
		return d.distillRsp, nil
	}

	return []interpreter.SkillAction{{Type: interpreter.SkillActionFinish}}, nil
}

func (d *v1MockInterpreter) Extract(ctx context.Context, history []v1.Message, messageWindow int, files []v1.File, currentTasks []v1.Task) ([]interpreter.TaskAction, error) {
	d.mtx.Lock()
	d.extractHistory = history
	d.extractFiles = files
	d.extractTasks = currentTasks
	d.mtx.Unlock()

	if len(d.extractRsp) > 0 {
		return d.extractRsp, nil
	}

	return []interpreter.TaskAction{{Type: interpreter.TaskActionFinish}}, nil
}

func (d *v1MockInterpreter) DistillHistory() []v1.Message {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.distillHistory
}

func (d *v1MockInterpreter) DistillSkills() []v1.Skill {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.distillSkills
}

func (d *v1MockInterpreter) ExtractHistory() []v1.Message {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.extractHistory
}

func (d *v1MockInterpreter) ExtractFiles() []v1.File {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.extractFiles
}

func (d *v1MockInterpreter) ExtractTasks() []v1.Task {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	return d.extractTasks
}

func NewV1Interpreter(opts ...interpreter.Option) *v1MockInterpreter {
	options := interpreter.NewOptions(opts...)

	d := &v1MockInterpreter{
		options:        options,
		distillHistory: []v1.Message{},
		distillSkills:  []v1.Skill{},
		extractHistory: []v1.Message{},
		extractFiles:   []v1.File{},
		extractTasks:   []v1.Task{},
		mtx:            sync.RWMutex{},
	}

	if rsp, ok := DistillRspFrom(options.Context); ok {
		d.distillRsp = rsp
	}

	if rsp, ok := ExtractRspFrom(options.Context); ok {
		d.extractRsp = rsp
	}

	return d
}
