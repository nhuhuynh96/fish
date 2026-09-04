.PHONY: up down logs restart build ps

DOCKER_BIN := $(shell which docker 2>/dev/null || echo "/Applications/Docker.app/Contents/Resources/bin/docker")

up:
	$(DOCKER_BIN) compose up -d --build

down:
	$(DOCKER_BIN) compose down

logs:
	$(DOCKER_BIN) compose logs -f

ps:
	$(DOCKER_BIN) compose ps

restart:
	$(DOCKER_BIN) compose restart
