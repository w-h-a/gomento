package v1mock

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/distiller"
)

type skillRspKey struct{}

func WithSkillRsp(rsp *v1.Skill) distiller.Option {
	return func(o *distiller.Options) {
		o.Context = context.WithValue(o.Context, skillRspKey{}, rsp)
	}
}

func SkillRspFrom(ctx context.Context) (*v1.Skill, bool) {
	rsp, ok := ctx.Value(skillRspKey{}).(*v1.Skill)
	return rsp, ok
}
