# go_statsd

This will be a feature complete port (minus some of the backends) of
[Etsy's statsd][1] written in Go. This is a heavily modified fork of
[Bitly's Implementation][2] which is in turn a fork of [amir/gographite][3].

This will be considered '1.0' when from a blackbox perspective the two pieces
of software are indistinguishable. From there improvements made by competing
third parties will be introduced.

Currently Supports:

* Counters (With both raw values and rates)
* Gauges (Does not support modifiers +/-)
* Sets
* Timers

The logic around stat aggregation is currently closer to Bitly's implementation
which is neither accurate, or compatible with the original statsd metrics. This
will be rapidly changing too reflect actual statistical measuring means.

# Installing

```bash
go get github.com/sstelfox/go_statsd
```

# Roadmap

In no particular order:

* Better test coverage, with conversions of original tests as applicable
* Configurable global stat prefix and type specific prefix
* Floating point percentiles
* Full gauges support (modifiers)
* Implement network compatible version of statsd management interface
* Make output statistics match the original Etsy implementation
* Packaged versions in common distributions

# Change Log

0.6.0:

* More documentation
* Added support for the Sets ('s') data type
* While not configurable, the default metric namespace now matches the default
  metrics namespaces of statsd (All stats are under the 'stats' namespace,
  counters are under 'stats.counters', timers under 'stats.timers', and so on.

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

# Contributing

I welcome and encourage pull requests. I receive a lot of GitHub notifications
on a daily basis, so please be a bit patient with a response too the pull
request (I'll do my best too respond within a couple of days).

Including a reason behind a pull request is mandatory for me too accept the
pull request, ideally with a description of the expected behavior change.

If you include meaningful tests for your pull request covering your changes or
any other piece of functionality in the code, I'll add you too this README as a
honored contributor (you'll show up in Github as a contributor either way).

Please make your pull requests against the develop branch, the master branch is
reserved for the latest tagged release.

# Motivations

A large majority of this project was looking for a public project written in Go
I could sink my teeth into. At the time of this project I was looking for an
alternative to statsd, as I have a personal distaste for NodeJS (A large amount
of impressive software has been written in this framework but it's not for me
or my servers).

The two objectives found harmony in what I consider an incomplete (though a
perfectly functional subset) implementation of statsd.

I am not writing this software with any commercial intent and more likely than
not I will never achieve the raw performance of Statsite's code base, but
comparitively this code base should be generally easier too test and reason
about than raw C code.

[1]: https://github.com/etsy/statsd
[2]: https://github.com/bitly/statsdaemon
[3]: https://github.com/amir/gographite

