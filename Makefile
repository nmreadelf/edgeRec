.PHONY: gofmt
## run gofmt format code
gofmt:
	gofmt -w -s ./
	goimports -local github.com/auxten/edgeRec -w ./
