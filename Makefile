DOCKER_IMAGE_NAME ?= uptimer

build:
	docker build -t $(DOCKER_IMAGE_NAME) .

tests:
	go test -v ./...
