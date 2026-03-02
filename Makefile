.PHONY: test lint vet build tidy deploy

test:
	go test -v ./...

lint:
	golangci-lint run

vet:
	go vet ./...

build:
	go build ./...

tidy:
	go mod tidy

deploy:
	gcloud functions deploy alchemy-webhook \
		--gen2 \
		--runtime=go125 \
		--region=asia-northeast1 \
		--source=. \
		--entry-point=AlchemyWebhook \
		--trigger-http \
		--allow-unauthenticated
