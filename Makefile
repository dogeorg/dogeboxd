default: build

.PHONY: clean, test
clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/enter_recovery_mode build/dbx build/_dbxroot build/chownpupstorage

build/dogeboxd: clean, mkbuild
	go build -o build/dogeboxd ./cmd/dogeboxd/. 

build/enter_recovery_mode: clean, mkbuild
	go build -o build/enter_recovery_mode ./cmd/enter_recovery_mode/.

build/dbx: clean, mkbuild
	go build -o build/dbx ./cmd/dbx/.

build/_dbxroot: clean, mkbuild
	go build -o build/_dbxroot ./cmd/_dbxroot/.

build/chownpupstorage: clean, mkbuild
	go build -o build/chownpupstorage ./cmd/chownpupstorage/.

dev:
	go run ./cmd/dogeboxd -v

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

orb:
	go run ./cmd/dogeboxd -v --addr 0.0.0.0 --danger-dev --data ~/data --nix ~/data/nix --port 3000 --uiport 8080

test:
	go test -v ./test
