API_CONTAINER_NAME=ara-api
API_SERVICE_NAME=api

.PHONY:

## build/api: build api application binary
build/api:
	@echo "Building server..."
	CGO_ENABLED=0 go build -o ./bin/ ./cmd/api

## start/api: run built api application binary
start/api: build/api
	@echo "Starting server..."
	./bin/api

## docker/start: run all applications in docker containers
docker/start:
	@echo "Starting server in docker..."
	docker compose up -d --build

## docker/logs/api: show logs for API container
docker/logs/api:
	docker compose logs -f $(API_SERVICE_NAME)

## docker/stop: stop all applications in docker containers
docker/stop:
	@echo "Stoping docker containers..."
	docker compose stop

## migration/up: run migrations
migration/up:
	@echo "Running migrations..."
	docker exec -it $(API_CONTAINER_NAME) goose -v up

## migration/down: rollback migrations
migration/down:
	@echo "Rolling back most recent migrations..."
	docker exec -it $(API_CONTAINER_NAME) goose -v down

## migration/reset: reset migrations
migration/reset:
	@echo "Resetting migrations..."
	docker exec -it $(API_CONTAINER_NAME) goose -v reset