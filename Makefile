.PHONY: app build check format keychain-integration release-dry-run test vet

build:
	scripts/build-binary.sh dist/key-session

app:
	scripts/build-app.sh "dist/Key Session.app"

format:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

test:
	go test -race ./...

vet:
	go vet ./...

check:
	scripts/ci.sh

keychain-integration:
	scripts/test-keychain-integration.sh

release-dry-run:
	scripts/build-release.sh dist
