package v1mock

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/persister"
)

type v1MockPersister struct {
	options  persister.Options
	projects map[uuid.UUID]*v1.Project
	spaces   map[uuid.UUID]*v1.Space
	sessions map[uuid.UUID]*v1.Session
	tasks    map[uuid.UUID]*v1.Task
	messages map[uuid.UUID][]v1.Message
	assets   map[uuid.UUID]*v1.Asset
	skills   map[uuid.UUID]*v1.Skill
	mtx      sync.RWMutex
}

func (p *v1MockPersister) CreateProject(ctx context.Context, proj *v1.Project) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.projects[proj.Id] = proj
	return nil
}

func (p *v1MockPersister) Projects() map[uuid.UUID]*v1.Project {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Project, len(p.projects))
	maps.Copy(cpy, p.projects)
	return cpy
}

func (p *v1MockPersister) CreateSpace(ctx context.Context, space *v1.Space) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.spaces[space.Id] = space
	return nil
}

func (p *v1MockPersister) Spaces() map[uuid.UUID]*v1.Space {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Space, len(p.spaces))
	maps.Copy(cpy, p.spaces)
	return cpy
}

func (p *v1MockPersister) CreateSession(ctx context.Context, sess *v1.Session) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sessions[sess.Id] = sess
	return nil
}

func (p *v1MockPersister) Sessions() map[uuid.UUID]*v1.Session {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Session, len(p.sessions))
	maps.Copy(cpy, p.sessions)
	return cpy
}

func (p *v1MockPersister) GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	if s, ok := p.sessions[id]; ok {
		return s, nil
	}
	return nil, nil
}

func (p *v1MockPersister) CreateTask(ctx context.Context, t *v1.Task) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	p.tasks[t.Id] = t
	return nil
}

func (p *v1MockPersister) UpdateTaskStatus(ctx context.Context, id uuid.UUID, status string) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	t, ok := p.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	t.Status = status
	t.UpdatedAt = time.Now()
	return nil
}

func (p *v1MockPersister) GetTask(ctx context.Context, id uuid.UUID) (*v1.Task, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	if t, ok := p.tasks[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (p *v1MockPersister) CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	existingMsgs := p.messages[msg.SessionId]
	if len(existingMsgs) > 0 {
		lastMsg := existingMsgs[len(existingMsgs)-1]
		msg.ParentId = &lastMsg.Id
	}

	for partIdx, a := range assets {
		if partIdx < len(msg.Parts) {
			msg.Parts[partIdx].AssetId = &a.Id
		}
		p.assets[a.Id] = a
	}

	p.messages[msg.SessionId] = append(p.messages[msg.SessionId], *msg)

	return nil
}

func (p *v1MockPersister) GetMessages(ctx context.Context, sessionId uuid.UUID, opts ...persister.GetMessagesOption) ([]v1.Message, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	msgs := p.messages[sessionId]
	cpy := make([]v1.Message, len(msgs))
	copy(cpy, msgs)
	return cpy, nil
}

func (p *v1MockPersister) GetAssets(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*v1.Asset, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Asset, len(p.assets))
	for _, id := range ids {
		if a, ok := p.assets[id]; ok {
			cpy[id] = a
		}
	}
	return cpy, nil
}

func (p *v1MockPersister) SaveSkill(ctx context.Context, skill *v1.Skill) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.skills[skill.Id] = skill
	return nil
}

func (p *v1MockPersister) Skills() map[uuid.UUID]*v1.Skill {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Skill, len(p.skills))
	maps.Copy(cpy, p.skills)
	return cpy
}

func NewV1Persister(opts ...persister.Option) *v1MockPersister {
	options := persister.NewOptions(opts...)

	p := &v1MockPersister{
		options:  options,
		projects: map[uuid.UUID]*v1.Project{},
		spaces:   map[uuid.UUID]*v1.Space{},
		sessions: map[uuid.UUID]*v1.Session{},
		tasks:    map[uuid.UUID]*v1.Task{},
		messages: map[uuid.UUID][]v1.Message{},
		assets:   map[uuid.UUID]*v1.Asset{},
		skills:   map[uuid.UUID]*v1.Skill{},
		mtx:      sync.RWMutex{},
	}

	return p
}
