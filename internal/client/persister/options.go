package persister

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Option func(*Options)

type Options struct {
	Location string
	Context  context.Context
}

func WithLocation(loc string) Option {
	return func(o *Options) {
		o.Location = loc
	}
}

func NewOptions(opts ...Option) Options {
	options := Options{
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}

type SortOrder int

const (
	SortOrderDesc SortOrder = iota
	SortOrderAsc
)

type GetMessagesOption func(*GetMessagesOptions)

type GetMessagesOptions struct {
	Limit          int
	Sort           SortOrder
	AfterCreatedAt time.Time
	AfterId        uuid.UUID // just for sort tie-breaking in cases where created at is identical
	Context        context.Context
}

func WithLimit(limit int) GetMessagesOption {
	return func(gmo *GetMessagesOptions) {
		gmo.Limit = limit
	}
}

func WithSort(sort SortOrder) GetMessagesOption {
	return func(gmo *GetMessagesOptions) {
		gmo.Sort = sort
	}
}

func WithAfterCreatedAt(afterCreatedAt time.Time) GetMessagesOption {
	return func(gmo *GetMessagesOptions) {
		gmo.AfterCreatedAt = afterCreatedAt
	}
}

func WithAfterId(id uuid.UUID) GetMessagesOption {
	return func(gmo *GetMessagesOptions) {
		gmo.AfterId = id
	}
}

func NewGetMessagesOptions(opts ...GetMessagesOption) GetMessagesOptions {
	options := GetMessagesOptions{
		Sort:    SortOrderDesc,
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}
