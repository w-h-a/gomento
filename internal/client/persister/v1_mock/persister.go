package v1mock

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/persister"
)

type v1MockPersister struct {
	options  persister.Options
	jobs     map[uuid.UUID]*v1.Job
	spaces   map[uuid.UUID]*v1.Space
	skills   map[uuid.UUID]*v1.Skill
	sessions map[uuid.UUID]*v1.Session
	tasks    map[uuid.UUID]*v1.Task
	messages map[uuid.UUID]*v1.Message
	assets   map[uuid.UUID]*v1.Asset
	files    map[uuid.UUID]*v1.File
	mtx      sync.RWMutex
}

func (p *v1MockPersister) CreateJob(ctx context.Context, job *v1.Job) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	cpy := *job
	p.jobs[job.Id] = &cpy
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

func (p *v1MockPersister) CreateSpace(ctx context.Context, space *v1.Space) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	cpy := *space
	p.spaces[space.Id] = &cpy
	return nil
}

func (p *v1MockPersister) ListSpaces(ctx context.Context) ([]v1.Space, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	var spaces []v1.Space
	for _, s := range p.spaces {
		cpy := *s
		spaces = append(spaces, cpy)
	}
	return spaces, nil
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
	cpy := *skill
	p.skills[skill.Id] = &cpy
	return nil
}

func (p *v1MockPersister) FetchCurrentSkills(ctx context.Context, spaceId uuid.UUID) ([]v1.Skill, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	cpy := make(map[uuid.UUID]*v1.Skill, len(p.skills))
	maps.Copy(cpy, p.skills)

	result := make([]v1.Skill, 0, len(cpy))
	for _, s := range cpy {
		if s.SpaceId == spaceId {
			result = append(result, *s)
		}
	}

	return result, nil
}

func (p *v1MockPersister) UpdateSkill(ctx context.Context, id uuid.UUID, trigger string, sop string, embedding []float32) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if existing, ok := p.skills[id]; ok {
		existing.Trigger = trigger
		existing.SOP = sop
		existing.Embedding = embedding
	}

	return nil
}

func (p *v1MockPersister) SearchSkills(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...persister.SearchOption) ([]v1.Skill, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	options := persister.NewSearchOptions(opts...)

	var results []v1.Skill
	for _, s := range p.skills {
		if s.SpaceId == spaceId {
			results = append(results, *s)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Id.String() < results[j].Id.String()
	})

	if options.Limit > 0 && len(results) > options.Limit {
		results = results[:options.Limit]
	}

	return results, nil
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
	cpy := *sess
	p.sessions[sess.Id] = &cpy
	return nil
}

func (p *v1MockPersister) ListSessions(ctx context.Context, spaceId *uuid.UUID) ([]v1.Session, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	var sessions []v1.Session
	for _, s := range p.sessions {
		if spaceId == nil {
			cpy := *s
			sessions = append(sessions, cpy)
		} else if s.SpaceId != nil && *s.SpaceId == *spaceId {
			cpy := *s
			sessions = append(sessions, cpy)
		}
	}

	return sessions, nil
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

func (p *v1MockPersister) UpdateSessionSpace(ctx context.Context, sess *v1.Session) error {
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

	msg.CreatedAt = time.Now()

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
		cpy := *a
		p.assets[a.Id] = &cpy
	}

	if p.messages == nil {
		p.messages = make(map[uuid.UUID]*v1.Message)
	}

	cpy := *msg
	if msg.Parts != nil {
		partsCpy := make([]v1.Part, len(msg.Parts))
		copy(partsCpy, msg.Parts)
		cpy.Parts = partsCpy
	}
	p.messages[msg.Id] = &cpy

	return nil
}

func (p *v1MockPersister) ListMessages(ctx context.Context, sessionId uuid.UUID, opts ...persister.ListMessagesOption) ([]v1.Message, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	options := persister.NewListMessagesOptions(opts...)

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

func (p *v1MockPersister) SearchMessages(ctx context.Context, spaceId uuid.UUID, vector []float32, opts ...persister.SearchOption) ([]v1.Message, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	options := persister.NewSearchOptions(opts...)

	var results []v1.Message
	for _, m := range p.messages {
		if sess, ok := p.sessions[m.SessionId]; ok {
			if sess.SpaceId != nil && *sess.SpaceId == spaceId {
				results = append(results, *m)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Id.String() < results[j].Id.String()
	})

	if options.Limit > 0 && len(results) > options.Limit {
		results = results[:options.Limit]
	}

	return results, nil
}

func (p *v1MockPersister) UpsertFileWithAsset(ctx context.Context, f *v1.File, a *v1.Asset) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	a.CreatedAt = time.Now()
	assetCpy := *a
	p.assets[a.Id] = &assetCpy

	f.AssetId = a.Id

	for _, existing := range p.files {
		if existing.SpaceId == f.SpaceId && existing.Path == f.Path && existing.Filename == f.Filename {
			existing.AssetId = f.AssetId
			existing.UpdatedAt = time.Now()
			*f = *existing
			return nil
		}
	}

	f.CreatedAt = time.Now()
	f.UpdatedAt = time.Now()

	fileCpy := *f
	p.files[f.Id] = &fileCpy

	return nil
}

func (p *v1MockPersister) ListFiles(ctx context.Context, spaceId *uuid.UUID, opts ...persister.ListFilesOption) ([]v1.File, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	options := persister.NewListFilesOptions(opts...)

	var files []v1.File
	for _, f := range p.files {
		if (f.SpaceId == nil && spaceId == nil) || (f.SpaceId != nil && spaceId != nil && *f.SpaceId == *spaceId) {
			if len(options.PathPrefix) > 0 {
				if !strings.HasPrefix(f.Path, options.PathPrefix) {
					continue
				}
			}

			fileCopy := *f

			if asset, ok := p.assets[f.AssetId]; ok {
				cpy := *asset
				fileCopy.Asset = &cpy
			}

			files = append(files, fileCopy)
		}
	}

	return files, nil
}

func (p *v1MockPersister) GetFile(ctx context.Context, id uuid.UUID) (*v1.File, error) {
	p.mtx.RLock()
	defer p.mtx.RUnlock()

	if f, ok := p.files[id]; ok {
		cpy := *f
		if asset, ok := p.assets[f.AssetId]; ok {
			assetRes := *asset
			cpy.Asset = &assetRes
		}
		return &cpy, nil
	}

	return nil, nil
}

func (p *v1MockPersister) UpdateFileSpace(ctx context.Context, file *v1.File) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if existing, ok := p.files[file.Id]; ok {
		existing.SpaceId = file.SpaceId
	}
	return nil
}

func (p *v1MockPersister) UpdateFileEmbedding(ctx context.Context, id uuid.UUID, vector []float32) error {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if existing, ok := p.files[id]; ok {
		existing.Embedding = vector
	}
	return nil
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

func NewV1Persister(opts ...persister.Option) *v1MockPersister {
	options := persister.NewOptions(opts...)

	p := &v1MockPersister{
		options:  options,
		jobs:     map[uuid.UUID]*v1.Job{},
		spaces:   map[uuid.UUID]*v1.Space{},
		skills:   map[uuid.UUID]*v1.Skill{},
		sessions: map[uuid.UUID]*v1.Session{},
		tasks:    map[uuid.UUID]*v1.Task{},
		messages: map[uuid.UUID]*v1.Message{},
		assets:   map[uuid.UUID]*v1.Asset{},
		files:    map[uuid.UUID]*v1.File{},
		mtx:      sync.RWMutex{},
	}

	return p
}
