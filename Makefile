BINS = client/peerdiscovery_query daemon/peerdiscovery_daemon
VERSION = 0.1
OS = $(shell uname -s)
ARCH = $(shell uname -m)
PACKAGE_NAME = scriptrock_peerdiscovery-$(VERSION)-$(OS)-$(ARCH)
TARBALL = $(PACKAGE_NAME).tar.gz
BUILD_DIR = build

default: $(BINS)

$(BINS): force
	(cd ${@D} && go get)
	(cd ${@D} && go build -o ${@F})

$(BUILD_DIR)/$(PACKAGE_NAME): $(BINS)
	@mkdir -p $@
	cp -a $^ $@

$(BUILD_DIR)/$(TARBALL): $(BUILD_DIR)/$(PACKAGE_NAME)
	(cd $< && tar cz ./*) | (cd ${@D} && cat > ${@F})

package: $(BUILD_DIR)/$(TARBALL)

clean: force
	rm -rf $(BUILD_DIR)/ $(BINS)

.PHONY: default package force clean


