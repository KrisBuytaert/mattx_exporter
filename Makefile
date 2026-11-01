.PHONY: test rpm test-rpm
test:
	go vet ./...
	go test -count=1 ./...

# Build the RPM into dist/ and check it.
rpm:
	OUT_DIR=$(CURDIR)/dist ./packaging/build-rpm.sh
	./packaging/test-rpm.sh $$(ls -t dist/mattx_exporter-*.x86_64.rpm | head -1)

test-rpm:
	./packaging/test-rpm.sh $$(ls -t dist/mattx_exporter-*.x86_64.rpm | head -1)
