# go-joplin Makefile
BINARY_NAME ?= go-joplin
IMAGE_NAME ?= ghcr.io/jescarri/gojoplin
VERSION ?= dev

.PHONY: test build docker-build docker-push

test:
	go test ./...

build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY_NAME) .

docker-build:
	docker build -t $(IMAGE_NAME):$(VERSION) .

docker-push: docker-build
	docker push $(IMAGE_NAME):$(VERSION)
