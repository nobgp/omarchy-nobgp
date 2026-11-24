VERSION ?= 0.2.0
NAME    := nobgp-omarchy
DIST    := build/$(NAME)-$(VERSION)

.PHONY: build test vet dist pkg clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o build/$(NAME) ./cmd/$(NAME)

test:
	go test ./...

vet:
	go vet ./...

# Source tarball for the PKGBUILD.
dist:
	rm -rf $(DIST) && mkdir -p $(DIST)
	cp -r go.mod go.sum cmd internal desktop packaging README.md Makefile $(DIST)/
	[ ! -f LICENSE ] || cp LICENSE $(DIST)/
	tar -C build -czf build/$(NAME)-$(VERSION).tar.gz $(NAME)-$(VERSION)
	rm -rf $(DIST)

# Build the Arch package (on Arch, as a normal user).
pkg: dist
	cp build/$(NAME)-$(VERSION).tar.gz packaging/archlinux/
	cd packaging/archlinux && makepkg -f

clean:
	rm -rf build packaging/archlinux/*.tar.gz packaging/archlinux/*.pkg.tar.zst packaging/archlinux/src packaging/archlinux/pkg
