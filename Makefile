SRCS := $(filter-out %_test.go, $(wildcard *.go cmd/actionlint/*.go)) go.mod go.sum .git-hooks/.timestamp
TESTS := $(filter %_test.go, $(wildcard *.go))
TOOL := $(filter %_test.go, $(wildcard scripts/*/*.go))
TESTDATA := $(wildcard \
		testdata/examples/* \
		testdata/err/* \
		testdata/ok/* \
		testdata/config/* \
		testdata/format/* \
		testdata/projects/* \
		testdata/reusable_workflow_metadata/* \
	)
GO_GEN_SRCS := scripts/generate-popular-actions/main.go \
				scripts/generate-popular-actions/popular_actions.json \
				scripts/generate-webhook-events/main.go \
				scripts/generate-availability/main.go

all: build test lint

.testtimestamp: $(TESTS) $(SRCS) $(TESTDATA) $(TOOL)
	go test ./...
	touch .testtimestamp

t test: .testtimestamp

.linttimestamp: $(TESTS) $(SRCS) $(TOOL) docs/checks.md
	go vet ./...
	staticcheck ./...
	GOOS=js GOARCH=wasm staticcheck ./playground
	go run ./scripts/check-checks -quiet ./docs/checks.md
	touch .linttimestamp

l lint: .linttimestamp

popular_actions.go all_webhooks.go availability.go: $(GO_GEN_SRCS)
ifdef SKIP_GO_GENERATE
	touch popular_actions.go all_webhooks.go availability.go
else
	go generate
endif

actionlint: $(SRCS)
	CGO_ENABLED=0 go build ./cmd/actionlint

b build: actionlint

actionlint_fuzz-fuzz.zip:
	go-fuzz-build ./fuzz

fuzz: actionlint_fuzz-fuzz.zip
	go-fuzz -bin ./actionlint_fuzz-fuzz.zip -func $(FUZZ_FUNC)

man/actionlint.1 man/actionlint.1.html: man/actionlint.1.ronn
	ronn man/actionlint.1.ronn

man: man/actionlint.1

bench:
	go test -bench Lint -benchmem

.github/actionlint-matcher.json: scripts/generate-actionlint-matcher/object.mjs
	node ./scripts/generate-actionlint-matcher/main.mjs .github/actionlint-matcher.json

scripts/generate-actionlint-matcher/test/escape.txt: actionlint
	./actionlint -color ./testdata/err/one_error.yaml > ./scripts/generate-actionlint-matcher/test/escape.txt || true
scripts/generate-actionlint-matcher/test/no_escape.txt: actionlint
	./actionlint -no-color ./testdata/err/one_error.yaml > ./scripts/generate-actionlint-matcher/test/no_escape.txt || true
scripts/generate-actionlint-matcher/test/want.json: actionlint
	./actionlint -format '{{json .}}' ./testdata/err/one_error.yaml > scripts/generate-actionlint-matcher/test/want.json || true

CHANGELOG.md: .bumptimestamp
	changelog-from-release > CHANGELOG.md

c clean:
	rm -f ./actionlint ./.testtimestamp ./.linttimestamp ./actionlint_fuzz-fuzz.zip ./man/actionlint.1 ./man/actionlint.1.html ./actionlint-workflow-ast
	rm -rf ./corpus ./crashers

.git-hooks/.timestamp: .git-hooks/pre-push
	[ -z "${CI}" ] && git config core.hooksPath .git-hooks || true
	touch .git-hooks/.timestamp

.PHONY: all test clean build lint fuzz man bench b t c l
