all: go_statsd
go_statsd:
	go build -ldflags "-s" go_statsd.go
clean:
	rm -f go_statsd
