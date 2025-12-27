package v1mock

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/interpreter"
)

type skillRspKey struct{}

func WithSkillRsp(rsp *v1.Skill) interpreter.Option {
	return func(o *interpreter.Options) {
		o.Context = context.WithValue(o.Context, skillRspKey{}, rsp)
	}
}

func SkillRspFrom(ctx context.Context) (*v1.Skill, bool) {
	rsp, ok := ctx.Value(skillRspKey{}).(*v1.Skill)
	return rsp, ok
}

type actionRspKey struct{}

func WithActionRsp(rsp []interpreter.TaskAction) interpreter.Option {
	return func(o *interpreter.Options) {
		o.Context = context.WithValue(o.Context, actionRspKey{}, rsp)
	}
}

func ActionRspFrom(ctx context.Context) ([]interpreter.TaskAction, bool) {
	rsp, ok := ctx.Value(actionRspKey{}).([]interpreter.TaskAction)
	return rsp, ok
}
