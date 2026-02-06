.PHONY: all clean dock-fire dock-fire-init install

all: dock-fire dock-fire-init

dock-fire:
	go build -o bin/dock-fire ./cmd/dock-fire

dock-fire-init:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/dock-fire-init -ldflags '-s -w -extldflags "-static"' ./cmd/dock-fire-init

install: all
	install -m 755 bin/dock-fire /usr/local/bin/dock-fire
	install -m 755 bin/dock-fire-init /usr/local/bin/dock-fire-init

clean:
	rm -rf bin/
