VERSION ?= latest

.PHONY: build
build:
	go build -o build/github-webhook-dco-labeler main.go

.PHONY: docker-build
docker-build:
	GOOS=linux GOARCH=amd64 go build -o build/github-webhook-dco-labeler *.go
	docker build -t quay.io/helmpack/github-webhook-dco-labeler:$(VERSION) .

.PHONY: docker-push
docker-push:
	docker push quay.io/helmpack/github-webhook-dco-labeler