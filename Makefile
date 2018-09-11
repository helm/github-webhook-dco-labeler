VERSION ?= latest

.PHONY: build
build:
	go build -o build/github-webhook-dco-labeler main.go

.PHONY: docker-build
docker-build:
	GOOS=linux GOARCH=amd64 go build -o build/github-webhook-dco-labeler *.go
	docker build -t mattfarina/github-webhook-dco-labeler:$(VERSION) .

.PHONY: docker-push
docker-push:
	docker push mattfarina/github-webhook-dco-labeler