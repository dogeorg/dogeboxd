default: build

.PHONY: clean, test
clean:
	rm -rf ./build

mkbuild:
	mkdir -p build/

build: build/dogeboxd build/enter_recovery_mode

build/dogeboxd: clean, mkbuild
	go build -o build/dogeboxd ./cmd/dogeboxd/. 

build/enter_recovery_mode: clean, mkbuild
	go build -o build/enter_recovery_mode ./cmd/enter_recovery_mode/.

dev:
	go run ./cmd/dogeboxd -v

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

test:
	go test -v ./test
