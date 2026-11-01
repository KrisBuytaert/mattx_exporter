.PHONY: test rpm test-rpm deb test-deb
test:
	go vet ./...
	go test -count=1 ./...

# Build the RPM into dist/ and check it.
rpm:
	OUT_DIR=$(CURDIR)/dist ./packaging/build-rpm.sh
	./packaging/test-rpm.sh $$(ls -t dist/mattx_exporter-*.x86_64.rpm | head -1)

test-rpm:
	./packaging/test-rpm.sh $$(ls -t dist/mattx_exporter-*.x86_64.rpm | head -1)

# Build the .deb into dist/ and check it.
deb:
	OUT_DIR=$(CURDIR)/dist ./packaging/build-deb.sh
	./packaging/test-deb.sh $$(ls -t dist/mattx-exporter_*_amd64.deb | head -1)

test-deb:
	./packaging/test-deb.sh $$(ls -t dist/mattx-exporter_*_amd64.deb | head -1)
