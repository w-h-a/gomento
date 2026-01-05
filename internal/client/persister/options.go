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

type ListMessagesOption func(*ListMessagesOptions)

type ListMessagesOptions struct {
	Limit          int
	Sort           SortOrder
	AfterCreatedAt time.Time
	AfterId        uuid.UUID // just for sort tie-breaking in cases where created at is identical
	Context        context.Context
}

func WithLimit(limit int) ListMessagesOption {
	return func(gmo *ListMessagesOptions) {
		gmo.Limit = limit
	}
}

func WithSort(sort SortOrder) ListMessagesOption {
	return func(gmo *ListMessagesOptions) {
		gmo.Sort = sort
	}
}

func WithAfterCreatedAt(afterCreatedAt time.Time) ListMessagesOption {
	return func(gmo *ListMessagesOptions) {
		gmo.AfterCreatedAt = afterCreatedAt
	}
}

func WithAfterId(id uuid.UUID) ListMessagesOption {
	return func(gmo *ListMessagesOptions) {
		gmo.AfterId = id
	}
}

func NewListMessagesOptions(opts ...ListMessagesOption) ListMessagesOptions {
	options := ListMessagesOptions{
		Sort:    SortOrderDesc,
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}

type ListFilesOption func(*ListFilesOptions)

type ListFilesOptions struct {
	PathPrefix string
	Context    context.Context
}

func WithPathPrefix(prefix string) ListFilesOption {
	return func(lfo *ListFilesOptions) {
		lfo.PathPrefix = prefix
	}
}

func NewListFilesOptions(opts ...ListFilesOption) ListFilesOptions {
	options := ListFilesOptions{
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}

type SearchOption func(*SearchOptions)

type SearchOptions struct {
	Limit   int
	Context context.Context
}

func SearchWithLimit(limit int) SearchOption {
	return func(so *SearchOptions) {
		so.Limit = limit
	}
}

func NewSearchOptions(opts ...SearchOption) SearchOptions {
	options := SearchOptions{
		Limit:   5,
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}
