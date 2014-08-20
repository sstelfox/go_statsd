# go_statsd

This will be a feature complete port (minus some of the backends) of
[Etsy's statsd][1] written in Go. This is a heavily modified fork of
[Bitly's Implementation][2] which is in turn a fork of [amir/gographite][3].

Currently Supports:

* Timing
* Counters (With both raw values and rates)
* Gauges (Does not support modifiers +/-)

The logic around stat aggregation is currently closer to Bitly's implementation
which is neither accurate, or compatible with the original statsd metrics. This
will be rapidly changing too reflect actual statistical measuring means.

# Installing

```bash
go get github.com/sstelfox/go_statsd
```

# Roadmap

* Implement network compatible version of statsd management interface
* Make output statistics match the original Etsy implementation
* Adjust internal logic around key names reflect the original implementation
* Add Sets data type
* Global stat prefix
* Full gauges support

# Change Log

0.5.5-alpha:

* Primarily code clean up, white space corrections, and more accurate variable
  and function names.
* No longer sending invalid '0' metrics for counts, instead just not connecting
  to graphite if there are no stats too send.
* Removed unecessary imports
* Made counters stat compatible with StatsD implemenation
* Added rates too counters
* Cleaned up unecessary complexity around counters

0.5.2-alpha:

* Last version forked from bitly's repository (Forked on 2014-08-18)

# Command Line Options

```
Usage of ./go_statsd:
  -a="::": The address too bind the server too (short hand).
  -address="::": The address too bind the server too.
  -flush-interval=10: Flush interval (seconds)
  -graphite="127.0.0.1:2003": Graphite service address (or - to disable)
  -percent-threshold=[]: Threshold percent (0-100, may be given multiple times)
  -port=8125: The UDP port too listen for metrics on.
  -receive-counter="statsd.count": Metric name for total metrics recevied per interval
  -version=false: Print version string and quit
```

[1]: https://github.com/etsy/statsd
[2]: https://github.com/bitly/statsdaemon
[3]: https://github.com/amir/gographite

