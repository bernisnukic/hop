.PHONY: test lint slo e2e down run

test:            ## Go tests (set HOP_TEST_DATABASE_URL to include Postgres)
	go vet ./... && go test -race ./...

lint:            ## Chart and Terraform checks
	helm lint charts/hop
	terraform fmt -check -recursive terraform

slo:             ## Unit-test the SLO alert rules with promtool
	docker run --rm -v "$(CURDIR):/w" -w /w --entrypoint promtool prom/prometheus:v3.8.0 test rules slo/tests.yaml

e2e:             ## kind cluster + platform + hop via Terraform, then the end-to-end checks
	scripts/e2e.sh

down:            ## Delete the kind cluster
	scripts/e2e.sh destroy

run:             ## Run hop locally with the in-memory store
	HOP_STORE=memory go run ./cmd/hop
