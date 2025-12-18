package server

type Server interface {
	Handle(handler any) error
	Run(stop chan struct{}) error
	Start() error
	Stop() error
}
