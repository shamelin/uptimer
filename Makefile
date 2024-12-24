DOCKER_IMAGE_NAME ?= uptime-seeker

build:
	docker build -t $(DOCKER_IMAGE_NAME) .

tests:
	go test -v ./...
