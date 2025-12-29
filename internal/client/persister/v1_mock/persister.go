package v1mock

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/persister"
)

type v1MockPersister struct {
	options   persister.Options
	jobs      map[uuid.UUID]*v1.Job
	projects  map[uuid.UUID]*v1.Project
	spaces    map[uuid.UUID]*v1.Space
	skills    map[uuid.UUID]*v1.Skill
	sessions  map[uuid.UUID]*v1.Session
	tasks     map[uuid.UUID]*v1.Task
	messages  map[uuid.UUID]*v1.Message
	assets    map[uuid.UUID]*v1.Asset
	artifacts map[uuid.UUID]*v1.Artifact
	files     map[uuid.UUID]*v1.File
	mtx       sync.RWMutex
}

func (p *v1MockPersister) CreateJob(ctx context.Context, job *v1.Job) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	p.jobs[job.Id] = job
	return nil
}

func (p *v1MockPersister) AcquireJobLock(ctx context.Context, jobId uuid.UUID) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	j, ok := p.jobs[jobId]
	if !ok {
		return fmt.Errorf("job not found")
	}

	if j.Status == v1.JobStatusRunning {
		return persister.ErrJobLocked
	}

	j.Status = v1.JobStatusRunning

	j.UpdatedAt = time.Now()

	return nil
}

func (p *v1MockPersister) UpdateJobStatus(ctx context.Context, id uuid.UUID, status string) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	j, ok := p.jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	j.Status = status
	j.UpdatedAt = time.Now()
	return nil
}

func (p *v1MockPersister) Jobs() map[uuid.UUID]*v1.Job {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	cpy := make(map[uuid.UUID]*v1.Job, len(p.jobs))
	maps.Copy(cpy, p.jobs)
	return cpy
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

func (p *v1MockPersister) GetSpace(ctx context.Context, id uuid.UUID) (*v1.Space, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	if s, ok := p.spaces[id]; ok {
		cpy := *s
		return &cpy, nil
	}
	return nil, nil
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

func (p *v1MockPersister) CreateSession(ctx context.Context, sess *v1.Session) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	p.sessions[sess.Id] = sess
	return nil
}

func (p *v1MockPersister) GetSession(ctx context.Context, id uuid.UUID) (*v1.Session, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	if s, ok := p.sessions[id]; ok {
		cpy := *s
		return &cpy, nil
	}
	return nil, nil
}

func (p *v1MockPersister) UpdateSession(ctx context.Context, sess *v1.Session) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if existing, ok := p.sessions[sess.Id]; ok {
		existing.SpaceId = sess.SpaceId
	}
	return nil
}

func (p *v1MockPersister) FetchCurrentTasks(ctx context.Context, sessionId uuid.UUID, status *string) ([]v1.Task, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	var result []v1.Task
	for _, t := range p.tasks {
		if t.SessionId == sessionId && !t.IsThought {
			if status != nil && t.Status != *status {
				continue
			}
			result = append(result, *t)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].TaskOrder < result[j].TaskOrder
	})

	return result, nil
}

func (p *v1MockPersister) InsertTask(ctx context.Context, sessionId uuid.UUID, afterOrder int, data []byte, status string) (*v1.Task, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, t := range p.tasks {
		if t.SessionId == sessionId && t.TaskOrder > afterOrder {
			t.TaskOrder++
		}
	}

	newTask := &v1.Task{
		Id:        uuid.New(),
		SessionId: sessionId,
		TaskOrder: afterOrder + 1,
		Data:      data,
		Status:    status,
		IsThought: false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	p.tasks[newTask.Id] = newTask

	return newTask, nil
}

func (p *v1MockPersister) UpdateTask(ctx context.Context, taskId uuid.UUID, status *string, order *int, data []byte) (*v1.Task, error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	t, ok := p.tasks[taskId]
	if !ok {
		return nil, persister.ErrTaskNotFound
	}

	if t.IsThought && order != nil {
		return nil, fmt.Errorf("cannot set task_order on a thought")
	}

	if order != nil && !t.IsThought && *order != t.TaskOrder {
		oldOrder := t.TaskOrder
		newOrder := *order

		for _, other := range p.tasks {
			if other.SessionId == t.SessionId && !other.IsThought && other.Id != t.Id {

				if newOrder > oldOrder {
					if other.TaskOrder > oldOrder && other.TaskOrder <= newOrder {
						other.TaskOrder--
					}
				}

				if newOrder < oldOrder {
					if other.TaskOrder >= newOrder && other.TaskOrder < oldOrder {
						other.TaskOrder++
					}
				}
			}
		}

		t.TaskOrder = newOrder
	}

	if status != nil {
		t.Status = *status
	}

	if data != nil {
		t.Data = data
	}

	t.UpdatedAt = time.Now()

	return t, nil
}

func (p *v1MockPersister) AppendMessagesToTask(ctx context.Context, taskId uuid.UUID, messageIds []uuid.UUID) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if _, ok := p.tasks[taskId]; !ok {
		return persister.ErrTaskNotFound
	}

	for _, msgId := range messageIds {
		if msg, ok := p.messages[msgId]; ok {
			msg.TaskId = &taskId
		}
	}

	return nil
}

func (p *v1MockPersister) AppendMessagesToThought(ctx context.Context, sessionId uuid.UUID, messageIds []uuid.UUID) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	var thought *v1.Task
	for _, t := range p.tasks {
		if t.SessionId == sessionId && t.IsThought {
			thought = t
			break
		}
	}

	if thought == nil {
		thought = &v1.Task{
			Id:        uuid.New(),
			SessionId: sessionId,
			TaskOrder: 0,
			IsThought: true,
			Status:    "pending",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Data:      []byte("{}"),
		}

		p.tasks[thought.Id] = thought
	}

	for _, msgId := range messageIds {
		if msg, ok := p.messages[msgId]; ok {
			msg.TaskId = &thought.Id
		}
	}

	return nil
}

func (p *v1MockPersister) CreateMessageWithAssets(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	var lastMsg *v1.Message
	for _, m := range p.messages {
		if m.SessionId == msg.SessionId {
			if lastMsg == nil || m.CreatedAt.After(lastMsg.CreatedAt) {
				lastMsg = m
			}
		}
	}

	if lastMsg != nil {
		msg.ParentId = &lastMsg.Id
		if !msg.CreatedAt.After(lastMsg.CreatedAt) {
			msg.CreatedAt = lastMsg.CreatedAt.Add(time.Nanosecond)
		}
	}

	for partIdx, a := range assets {
		if partIdx < len(msg.Parts) {
			msg.Parts[partIdx].AssetId = &a.Id
		}
		p.assets[a.Id] = a
	}

	if p.messages == nil {
		p.messages = make(map[uuid.UUID]*v1.Message)
	}

	p.messages[msg.Id] = msg

	return nil
}

func (p *v1MockPersister) GetMessages(ctx context.Context, sessionId uuid.UUID, opts ...persister.GetMessagesOption) ([]v1.Message, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	options := persister.NewGetMessagesOptions(opts...)

	var sessionMsgs []v1.Message
	for _, m := range p.messages {
		if m.SessionId == sessionId {
			sessionMsgs = append(sessionMsgs, *m)
		}
	}

	sort.Slice(sessionMsgs, func(i, j int) bool {
		if options.Sort == persister.SortOrderAsc {
			// return older first
			return sessionMsgs[i].CreatedAt.Before(sessionMsgs[j].CreatedAt)
		}
		// return newer first
		return sessionMsgs[i].CreatedAt.After(sessionMsgs[j].CreatedAt)
	})

	if options.Limit > 0 && len(sessionMsgs) > options.Limit {
		sessionMsgs = sessionMsgs[:options.Limit]
	}

	return sessionMsgs, nil
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

func (p *v1MockPersister) CreateArtifact(ctx context.Context, a *v1.Artifact) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	a.CreatedAt = time.Now()
	a.UpdatedAt = time.Now()
	p.artifacts[a.Id] = a
	return nil
}

func (p *v1MockPersister) CreateAsset(ctx context.Context, a *v1.Asset) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, existing := range p.assets {
		if existing.Container == a.Container && existing.Path == a.Path {
			existing.ETag = a.ETag
			existing.SHA256 = a.SHA256
			existing.MIME = a.MIME
			existing.SizeBytes = a.SizeBytes
			*a = *existing
			return nil
		}
	}

	a.CreatedAt = time.Now()

	p.assets[a.Id] = a

	return nil
}

func (p *v1MockPersister) UpsertFile(ctx context.Context, f *v1.File) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	for _, existing := range p.files {
		if existing.ArtifactId == f.ArtifactId && existing.Path == f.Path && existing.Filename == f.Filename {
			existing.AssetId = f.AssetId
			existing.UpdatedAt = time.Now()
			*f = *existing
			return nil
		}
	}

	f.CreatedAt = time.Now()
	f.UpdatedAt = time.Now()

	p.files[f.Id] = f

	return nil
}

func (p *v1MockPersister) ListFiles(ctx context.Context, artifactId uuid.UUID) ([]v1.File, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	var files []v1.File
	for _, f := range p.files {
		if f.ArtifactId == artifactId {
			fileCopy := *f
			if asset, ok := p.assets[f.AssetId]; ok {
				fileCopy.Asset = asset
			}
			files = append(files, fileCopy)
		}
	}
	return files, nil
}

func NewV1Persister(opts ...persister.Option) *v1MockPersister {
	options := persister.NewOptions(opts...)

	p := &v1MockPersister{
		options:   options,
		jobs:      map[uuid.UUID]*v1.Job{},
		projects:  map[uuid.UUID]*v1.Project{},
		spaces:    map[uuid.UUID]*v1.Space{},
		skills:    map[uuid.UUID]*v1.Skill{},
		sessions:  map[uuid.UUID]*v1.Session{},
		tasks:     map[uuid.UUID]*v1.Task{},
		messages:  map[uuid.UUID]*v1.Message{},
		assets:    map[uuid.UUID]*v1.Asset{},
		artifacts: map[uuid.UUID]*v1.Artifact{},
		files:     map[uuid.UUID]*v1.File{},
		mtx:       sync.RWMutex{},
	}

	return p
}
