export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0

.PHONY: build deploy

all: build

build: build-controller build-worker

build-controller:
	go build -o build/_output/bin/chaos-controller ./cmd/controller
	docker build . -f build/controller/Dockerfile -t atomix/chaos-controller:latest

build-worker:
	go build -o build/_output/bin/chaos-worker ./cmd/worker
	docker build . -f build/worker/Dockerfile -t atomix/chaos-worker:latest

push: push-controller push-worker

push-controller:
	docker push atomix/chaos-controller:latest

push-worker:
	docker push atomix/chaos-worker:latest

control:
	WATCH_NAMESPACE=default OPERATOR_NAME=chaos-controller go run cmd/controller/main.go

work:
	WATCH_NAMESPACE=default OPERATOR_NAME=chaos-worker NODE_NAME=server1 go run cmd/worker/main.go