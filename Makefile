default: build

.PHONY: clean mkbuild build multipassdev dpanel-build dev recovery test dbxdev sync-api

DPANEL_DIR ?= ../dpanel
# dpanel is built via its Nix package (the same derivation the OS image
# ships), so npm never runs inside the shared checkout and the host's
# node_modules (shared between macOS and the Linux VM under OrbStack) is
# never clobbered with binaries for the wrong platform. The result symlink
# points into the Nix store, where the package's $out is the dist directory.
DPANEL_DIST ?= build/dpanel

clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/dbx build/_dbxroot

build/dogeboxd: clean mkbuild
	go build \
		-o build/dogeboxd \
			./cmd/dogeboxd/.

build/dbx: clean mkbuild
	go build \
		-o build/dbx \
		./cmd/dbx/.

build/_dbxroot: clean mkbuild
	go build \
		-o build/_dbxroot \
		./cmd/_dbxroot/.

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

# Build dpanel with its own Nix package (the same one the OS image ships),
# overriding the pinned dogeboxd source with this checkout so the generated
# protos match the daemon being run. git+file refs build from the working
# tree's *tracked* files: uncommitted edits are included, but brand-new files
# must be `git add`ed before they appear in the build.
dpanel-build: mkbuild
	@command -v nix >/dev/null 2>&1 || { \
		echo "error: nix is required to build dpanel (see $(DPANEL_DIR)/flake.nix)" >&2; \
		exit 127; \
	}
	nix build "git+file://$(realpath $(DPANEL_DIR))" \
		--override-input dogeboxd-src "git+file://$(CURDIR)" \
		--no-write-lock-file \
		-o $(DPANEL_DIST)

dev: build dpanel-build
	/run/wrappers/bin/dogeboxd -v --addr 0.0.0.0 --danger-dev \
		--data ~/data --nix ~/data/nix --containerlogdir ~/data/containerlogs \
		--port 3000 --uiport 8080 --uidir $(DPANEL_DIST) \
		--unix-socket ~/data/dbx-socket $(ARGS)

recovery:
	ARGS=--force-recovery make dev

test:
	go test -v ./test

create-loop-device:
	sudo truncate -s 512000000000 /loop0.img && sudo losetup /dev/loop0 /loop0.img

create-loop-device-2:
	sudo truncate -s 512000000000 /loop1.img && sudo losetup /dev/loop1 /loop1.img

delete-loop-device:
	sudo losetup -d /dev/loop0 && sudo rm /loop0.img

delete-loop-device-2:
	sudo losetup -d /dev/loop1 && sudo rm /loop1.img

dbxdev:
	DEV_DIR=~/data/dev DBX_SOCKET=~/data/dbx-socket DBX_CONTAINER_LOG_DIR=~/data/containerlogs go run ./cmd/dbx dev

.PHONY: dbxsetup

dbxsetup:
	DEV_SEED_WORD_INDEX=1 DEV_DIR=~/data/dev DBX_SOCKET=~/data/dbx-socket go run ./cmd/dbx setup

# Regenerate Go code from the protobuf schemas in ./protocol
sync-api:
	cd protocol && buf generate
