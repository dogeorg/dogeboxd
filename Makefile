default: build/dogeboxd

.PHONY: clean, test
clean:
	rm -rf ./build

build/dogeboxd: clean
	mkdir -p build/
	go build -o build/dogeboxd ./cmd/dogeboxd/. 


dev:
	go run ./cmd/dogeboxd -v

multipassdev:
	go run ./cmd/dogeboxd -v -addr 0.0.0.0 -pups ~/

test:
	go test -v ./test
