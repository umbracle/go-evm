
.PHONY: download-spec-tests
download-spec-tests:
	git submodule init
	git submodule update

.PHONY: tests
tests:
	go test -v ./... -test.short
