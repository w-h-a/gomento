.PHONY: tidy
tidy:
	go mod tidy

.PHONY: style
style:
	goimports -l -w ./

.PHONY: unit-test
unit-test:
	go clean -testcache && go test -v ./...

.PHONY: integration-test
integration-test:
	go clean -testcache && INTEGRATION=1 go test -v ./...

.PHONY: go-build
go-build:
	CGO_ENABLED=0 go build -o ./bin/gomento ./cmd/gomento

.PHONY: go-install
go-install:
	go install

DB_URL=postgres://user:password@localhost:5432/gomento?sslmode=disable

.PHONY: migrate-up migrate-down migrate-new

migrate-up:
	migrate -path db/migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path db/migrations -database "$(DB_URL)" down 1

migrate-new:
	migrate create -ext sql -dir db/migrations -seq $(name)

pg-connect:
	docker exec -it gomento-pg psql -U user -d gomento