.PHONY: build mod

mod:
	go mod download
	go mod tidy
