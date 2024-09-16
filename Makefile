default: build

.PHONY: clean, test
clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/dbx build/_dbxroot

build/dogeboxd: clean, mkbuild
	go build -o build/dogeboxd ./cmd/dogeboxd/. 

build/dbx: clean, mkbuild
	go build -o build/dbx ./cmd/dbx/.

build/_dbxroot: clean, mkbuild
	go build -o build/_dbxroot ./cmd/_dbxroot/.

dev:
	go run ./cmd/dogeboxd -v

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

orb:
	go run ./cmd/dogeboxd -v --addr 0.0.0.0 --danger-dev --data ~/data --nix ~/data/nix --port 3000 --uiport 8080

test:
	go test -v ./test
