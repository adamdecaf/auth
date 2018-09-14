VERSION := $(shell grep -Eo '(v[0-9]+[\.][0-9]+[\.][0-9]+(-dev)?)' main.go)

.PHONY: build docker release

build:
	go fmt ./...
	CGO_ENABLED=1 go build -o bin/auth .

client:
# Download
	if [ ! -d "$(shell pwd)/tmp/swagger-codegen" ]; then \
		git clone https://github.com/swagger-api/swagger-codegen tmp/swagger-codegen; \
	fi
	cd tmp/swagger-codegen && \
	mvn clean package && \
	java -jar tmp/swagger-codegen/modules/swagger-codegen-cli/target/swagger-codegen-cli.jar \
	  generate -i openapi.yaml -l go -o client/

docker:
	docker build -t moov.io/auth:$(VERSION) -f Dockerfile .

release: docker
	CGO_ENABLED=0 go vet ./...
	CGO_ENABLED=0 go test ./...
	git tag $(VERSION)

release-push:
	git push origin $(VERSION)
