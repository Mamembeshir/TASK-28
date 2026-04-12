.PHONY: serve down migrate seed test test-unit test-integration test-frontend coverage shell fresh env css

# Copy .env.example → .env if .env is missing
env:
	@if [ ! -f .env ] && [ -f .env.example ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	fi

css:
	npx tailwindcss@3.4.4 -i static/css/input.css -o static/css/app.css

serve: env css
	docker-compose up

down:
	docker-compose down

migrate:
	docker-compose exec app ./eduexchange migrate

seed:
	docker-compose exec app ./eduexchange seed

# All tests (runs unit + frontend + integration inside Docker)
test:
	bash run_tests.sh

test-unit:
	bash run_tests.sh --unit

test-integration:
	bash run_tests.sh --integration

test-frontend:
	bash run_tests.sh --frontend

coverage:
	bash run_tests.sh --coverage

# Legacy alias for test-integration
test-api: test-integration

shell:
	docker-compose exec app sh

fresh: env
	docker-compose exec app ./eduexchange migrate-fresh --seed
