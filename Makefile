.PHONY: css test vet build

# Compile the tailwind stylesheet into the embedded assets. Run inside the dev
# container: docker run --rm -v "$$PWD:/app" -w /app simplearchive-dev make css
css:
	./scripts/build-css.sh

# Run tests inside the dev container.
test:
	go test ./...

vet:
	go vet ./...

build:
	go build .
