default: build

.PHONY: clean test
clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/dbx build/_dbxroot

build/dogeboxd: clean mkbuild
	go build \
		-ldflags "-X github.com/dogeorg/dogeboxd/pkg/version.dbxRelease=${DBX_RELEASE} -X version.nurHash=${DBX_NUR_HASH}" \
		-o build/dogeboxd \
			./cmd/dogeboxd/.

build/dbx: clean mkbuild
	go build \
		-ldflags "-X github.com/dogeorg/dogeboxd/pkg/version.dbxRelease=${DBX_RELEASE} -X github.com/dogeorg/dogeboxd/pkg/version.nurHash=${DBX_NUR_HASH}" \
		-o build/dbx \
		./cmd/dbx/.

build/_dbxroot: clean mkbuild
	go build \
		-ldflags "-X github.com/dogeorg/dogeboxd/pkg/version.dbxRelease=${DBX_RELEASE} -X github.com/dogeorg/dogeboxd/pkg/version.nurHash=${DBX_NUR_HASH}" \
		-o build/_dbxroot \
		./cmd/_dbxroot/.

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

dev:
	make && /run/wrappers/bin/dogeboxd -v --addr 0.0.0.0 --danger-dev --data ~/data --nix ~/data/nix --containerlogdir ~/data/containerlogs --port 3000 --uiport 8080 $(ARGS)

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
