default: build/dogeboxd

.PHONY: clean, test
clean:
	rm -rf ./build

build/dogeboxd: clean
	mkdir -p build/
	go build -o build/dogeboxd ./cmd/dogeboxd/. 


dev:
	go run ./cmd/dogeboxd -v 


test:
	go test -v ./test
