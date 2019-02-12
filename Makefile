all:
	make clean tx rx
proto:
	protoc --go_out=. proto/*.proto
tx:
	go build tx.go common.go
rx:
	go build rx.go common.go
clean:
	rm -f tx rx
