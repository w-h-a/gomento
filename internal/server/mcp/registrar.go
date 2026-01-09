package mcp

type Registrar interface {
	Handle(handler any) error
}
