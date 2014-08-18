# go_statsd

Clean-up and feature complete port (minus various backends) of [Etsy's
statsd][1] written in Go. This is a fork of [Bitly's Implementation][2] which
is in turn a fork of another repo [amir/gographite][3].

Supports:

* Timing
* Counters
* Gauges

# Installing

```bash
go get github.com/sstelfox/go_statsd
```

# Change Log

0.5.5-alpha:

* Primarily code clean up, white space corrections, and more accurate variable names

0.5.2-alpha:

* Last version forked from bitly's repository (Forked on 2014-08-18)

# Command Line Options

```
Usage of ./statsdaemon:
  -address=":8125": UDP service address
  -debug=false: print statistics sent to graphite
  -flush-interval=10: Flush interval (seconds)
  -graphite="127.0.0.1:2003": Graphite service address (or - to disable)
  -percent-threshold=[]: Threshold percent (0-100, may be given multiple times)
  -persist-count-keys=60: number of flush-interval's to persist count keys
  -receive-counter="": Metric name for total metrics recevied per interval
  -version=false: print version string
```

[1]: https://github.com/etsy/statsd
[2]: https://github.com/bitly/statsdaemon
[3]: https://github.com/amir/gographite
