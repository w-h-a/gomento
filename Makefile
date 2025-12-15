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
	CGO_ENABLED=0 go build -o ./bin/gomento ./

.PHONY: go-install
go-install:
	go install