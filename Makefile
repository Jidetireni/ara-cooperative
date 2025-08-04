.PHONY:

run:
	go run ./cmd/api

migrate/create:
	goose -s -v create $(name) sql

migrate/up:
	goose -v up

migrate/down:
	goose -v down

migrate/reset:
	goose -v reset
