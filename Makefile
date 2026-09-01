.PHONY: all build run test bench clean docker-build

APP_NAME := linkup
CMD_DIR := ./cmd/linkup
BUILD_DIR := ./bin

all: test build

build:
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

run:
	go run $(CMD_DIR)

test:
	go test -v -race ./...

bench:
	go test -bench=. -benchmem ./...

clean:
	rm -rf $(BUILD_DIR) *.db *.db-journal *.db-wal *.db-shm data/

docker-build:
	docker build -t kaicorplabs/linkup:latest .
