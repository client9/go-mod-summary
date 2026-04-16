
go-mod-summary: *.go
	go build ./...

test:
	go test ./...
clean:
	rm go-mod-summary

