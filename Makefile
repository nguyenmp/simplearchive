.PHONY: css test vet build dev-image

# Build the dev image locally:
#   make dev-image
# Then compile CSS inside it:
#   docker run --rm -v "$PWD:/app" -w /app simplearchive-dev make css
dev-image:
	docker build --target dev -t simplearchive-dev .

# Compile the tailwind stylesheet into the embedded assets.
css:
	./scripts/build-css.sh

# Run tests inside the dev container.
test:
	go test ./...

vet:
	go vet ./...

build:
	go build .
