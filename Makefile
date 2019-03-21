LDFLAGS_VERSION=-X main.txwxVersion=`git describe --tags --abbrev=0` -X main.txwxBuild=`git log -n 1 --pretty=%H`
BUILDINFO=-ldflags "$(LDFLAGS_VERSION)"

all:
	make clean tx rx
proto:
	protoc --go_out=. proto/*.proto
tx:
	go build $(BUILDINFO) -p 4 tx.go common.go
rx:
	go build $(BUILDINFO) -p 4 rx.go common.go
clean:
	rm -f tx rx
