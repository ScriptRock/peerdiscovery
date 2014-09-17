BUILD_DIR = build
BINS = build/peerdiscovery_etcd_conf build/peerdiscovery_daemon
VERSION = 0.1.0
OS = $(shell uname -s)
ARCH = $(shell uname -m)
PACKAGE_NAME = scriptrock_peerdiscovery-$(VERSION)-$(OS)-$(ARCH)
TARBALL = $(PACKAGE_NAME).tar.gz

GITHUB_RELEASE_URL = https://uploads.github.com/repos/ScriptRock/peerdiscovery/releases

default: $(BINS)

$(BINS): force

$(BUILD_DIR)/%: %/main.go
	(cd ${<D} && go get)
	(cd ${<D} && go build -o ../$@)

$(BUILD_DIR)/$(PACKAGE_NAME): $(BINS)
	@mkdir -p $@
	cp -a $^ $@

$(BUILD_DIR)/$(TARBALL): $(BUILD_DIR)/$(PACKAGE_NAME)
	(cd $< && tar cz ./*) | (cd ${@D} && cat > ${@F})

package: $(BUILD_DIR)/$(TARBALL)

goxc_coreos:
	goxc -bc="linux,amd64" -pv coreos -d=build/

goxc:
	goxc -bc="linux,!arm, darwin,!arm" -pv $(VERSION) -d=build/

clean: force
	rm -rf $(BUILD_DIR)/ $(BINS)

push_package_to_github: $(BUILD_DIR)/$(TARBALL)
	curl -H "Content-Type: application/x-compressed" --upload-file $< $(GITHUB_RELEASE_URL)/$(VERSION)/assets?name=${<F}

.PHONY: default package force clean


