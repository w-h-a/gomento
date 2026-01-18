package v1mock

import (
	"context"

	"github.com/w-h-a/gomento/internal/client/interpreter"
)

type distillRspKey struct{}

func WithDistillRsp(rsp []interpreter.SkillAction) interpreter.Option {
	return func(o *interpreter.Options) {
		o.Context = context.WithValue(o.Context, distillRspKey{}, rsp)
	}
}

func DistillRspFrom(ctx context.Context) ([]interpreter.SkillAction, bool) {
	rsp, ok := ctx.Value(distillRspKey{}).([]interpreter.SkillAction)
	return rsp, ok
}

type extractRspKey struct{}

func WithExtractRsp(rsp []interpreter.TaskAction) interpreter.Option {
	return func(o *interpreter.Options) {
		o.Context = context.WithValue(o.Context, extractRspKey{}, rsp)
	}
}

func ExtractRspFrom(ctx context.Context) ([]interpreter.TaskAction, bool) {
	rsp, ok := ctx.Value(extractRspKey{}).([]interpreter.TaskAction)
	return rsp, ok
}
