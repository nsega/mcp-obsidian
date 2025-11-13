.PHONY: build clean install test

build:
	go build -o mcp-obsidian .

clean:
	rm -f mcp-obsidian

install:
	go install

test:
	go test -v ./...

run:
	@echo "Usage: make run VAULT=/path/to/vault"
	@if [ -z "$(VAULT)" ]; then \
		echo "Error: VAULT parameter is required"; \
		exit 1; \
	fi
	@./mcp-obsidian $(VAULT)
