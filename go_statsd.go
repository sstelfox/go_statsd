
package main

import (
  "bytes"
  "errors"
  "flag"
  "fmt"
  "log"
  "math"
  "net"
  "os"
  "os/signal"
  "regexp"
  "sort"
  "strconv"
  "strings"
  "syscall"
  "time"
)

const (
  VERSION                 = "0.5.5-alpha"
  MAX_UNPROCESSED_PACKETS = 2048
  MAX_UDP_PACKET_SIZE     = 784
)

var signalChannel chan os.Signal

type StatSample struct {
  Bucket   string
  Value    interface{}
  Modifier string
  SampleRate float32
}

type Uint64Slice []uint64
func (s Uint64Slice) Len() int           { return len(s) }
func (s Uint64Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s Uint64Slice) Less(i, j int) bool { return s[i] < s[j] }

type Percentile struct {
  float float64
  str   string
}

func (p *Percentile) String() string { return p.str }

type Percentiles []*Percentile

func (a *Percentiles) String() string { return fmt.Sprintf("%v", *a) }
func (a *Percentiles) Set(s string) error {
  f, err := strconv.ParseFloat(s, 64)

  if err != nil {
    return err
  }

  *a = append(*a, &Percentile{f, strings.Replace(s, ".", "_", -1)})

  return nil
}

var (
  StatPipe = make(chan *StatSample, MAX_UNPROCESSED_PACKETS)
  counters = make(map[string]int64)
  gauges   = make(map[string]uint64)
  timers   = make(map[string]Uint64Slice)
)

func startCollector() {
  period := time.Duration(flushInterval) * time.Second
  ticker := time.NewTicker(period)
  for {
    select {
    case sig := <-signalChannel:
      log.Printf("!! Caught signal %d... shutting down\n", sig)
      if err := publishAggregates(time.Now().Add(period)); err != nil {
        log.Printf("ERROR: %s", err)
      }
      return
    case <-ticker.C:
      if err := publishAggregates(time.Now().Add(period)); err != nil {
        log.Printf("ERROR: %s", err)
      }
    case s := <-StatPipe:
      if (receiveCounter != "") {
        v, ok := counters[receiveCounter]
        if !ok || v < 0 {
          counters[receiveCounter] = 0
        }
        counters[receiveCounter] += 1
      }

      if s.Modifier == "ms" {
        _, ok := timers[s.Bucket]
        if !ok {
          var t Uint64Slice
          timers[s.Bucket] = t
        }
        timers[s.Bucket] = append(timers[s.Bucket], s.Value.(uint64))
      } else if s.Modifier == "g" {
        gauges[s.Bucket] = s.Value.(uint64)
      } else {
        v, ok := counters[s.Bucket]
        if !ok || v < 0 {
          counters[s.Bucket] = 0
        }
        counters[s.Bucket] += int64(float64(s.Value.(int64)) * float64(1/s.SampleRate))
      }
    }
  }
}

func publishAggregates(deadline time.Time) error {
  var buffer bytes.Buffer
  var num int64

  now := time.Now().Unix()

  client, err := net.Dial("tcp", graphiteAddress)
  if err != nil {
    if debug {
      log.Printf("WARNING: resetting counters when in debug mode")
      processCounters(&buffer, now)
      processGauges(&buffer, now)
      processTimers(&buffer, now, percentThreshold)
    }
    errmsg := fmt.Sprintf("dialing %s failed - %s", graphiteAddress, err)
    return errors.New(errmsg)
  }
  defer client.Close()

  err = client.SetDeadline(deadline)
  if err != nil {
    errmsg := fmt.Sprintf("could not set deadline:", err)
    return errors.New(errmsg)
  }

  num += processCounters(&buffer, now)
  num += processGauges(&buffer, now)
  num += processTimers(&buffer, now, percentThreshold)
  if num == 0 {
    return nil
  }

  if debug {
    for _, line := range bytes.Split(buffer.Bytes(), []byte("\n")) {
      if len(line) == 0 {
        continue
      }
      log.Printf("DEBUG: %s", line)
    }
  }

  _, err = client.Write(buffer.Bytes())
  if err != nil {
    errmsg := fmt.Sprintf("failed to write stats - %s", err)
    return errors.New(errmsg)
  }

  log.Printf("sent %d stats to %s", num, graphiteAddress)

  return nil
}

func processCounters(buffer *bytes.Buffer, now int64) int64 {
  var num int64
  // continue sending zeros for counters for a short period of time even if we have no new data
  // note we use the same in-memory value to denote both the actual value of the counter (value >= 0)
  // as well as how many turns to keep the counter for (value < 0)
  // for more context see https://github.com/bitly/statsdaemon/pull/8
  for s, c := range counters {
    switch {
    case c <= persistCountKeys:
      // consider this purgable
      delete(counters, s)
      continue
    case c < 0:
      counters[s] -= 1
      fmt.Fprintf(buffer, "%s %d %d\n", s, 0, now)
    case c >= 0:
      counters[s] = -1
      fmt.Fprintf(buffer, "%s %d %d\n", s, c, now)
    }
    num++
  }
  return num
}

func processGauges(buffer *bytes.Buffer, now int64) int64 {
  var num int64

  for g, c := range gauges {
    if c == math.MaxUint64 {
      continue
    }

    fmt.Fprintf(buffer, "%s %d %d\n", g, c, now)
    gauges[g] = math.MaxUint64
    num++
  }

  return num
}

func processTimers(buffer *bytes.Buffer, now int64, pctls Percentiles) int64 {
  var num int64

  for u, t := range timers {
    if len(t) > 0 {
      num++

      sort.Sort(t)
      min := t[0]
      max := t[len(t)-1]
      maxAtThreshold := max
      count := len(t)

      sum := uint64(0)
      for _, value := range t {
        sum += value
      }
      mean := float64(sum) / float64(len(t))

      for _, pct := range pctls {

        if len(t) > 1 {
          var abs float64
          if pct.float >= 0 {
            abs = pct.float
          } else {
            abs = 100 + pct.float
          }
          // poor man's math.Round(x):
          // math.Floor(x + 0.5)
          indexOfPerc := int(math.Floor(((abs / 100.0) * float64(count)) + 0.5))
          if pct.float >= 0 {
            indexOfPerc -= 1 // index offset=0
          }
          maxAtThreshold = t[indexOfPerc]
        }

        var tmpl string
        var pctstr string
        if pct.float >= 0 {
          tmpl = "%s.upper_%s %d %d\n"
          pctstr = pct.str
        } else {
          tmpl = "%s.lower_%s %d %d\n"
          pctstr = pct.str[1:]
        }
        fmt.Fprintf(buffer, tmpl, u, pctstr, maxAtThreshold, now)
      }

      var z Uint64Slice
      timers[u] = z

      fmt.Fprintf(buffer, "%s.mean %f %d\n", u, mean, now)
      fmt.Fprintf(buffer, "%s.upper %d %d\n", u, max, now)
      fmt.Fprintf(buffer, "%s.lower %d %d\n", u, min, now)
      fmt.Fprintf(buffer, "%s.count %d %d\n", u, count, now)
    }
  }

  return num
}

var packetRegexp = regexp.MustCompile("^([^:]+):(-?[0-9]+)\\|(g|c|ms)(\\|@([0-9\\.]+))?\n?$")

func parseMessages(data []byte) []*StatSample {
  var output []*StatSample

  for _, line := range bytes.Split(data, []byte("\n")) {
    if len(line) == 0 {
      continue
    }

    item := packetRegexp.FindSubmatch(line)
    if len(item) == 0 {
      continue
    }

    var err error
    var value interface{}

    modifier := string(item[3])

    switch modifier {
      case "c":
        value, err = strconv.ParseInt(string(item[2]), 10, 64)
        if err != nil {
          log.Printf("ERROR: failed to ParseInt %s - %s", item[2], err)
          continue
        }
      default:
        value, err = strconv.ParseUint(string(item[2]), 10, 64)
        if err != nil {
          log.Printf("ERROR: failed to ParseUint %s - %s", item[2], err)
          continue
        }
    }

    sampleRate, err := strconv.ParseFloat(string(item[5]), 32)
    if err != nil {
      sampleRate = 1
    }

    stat := &StatSample{
      Bucket:   string(item[1]),
      Value:    value,
      Modifier: modifier,
      SampleRate: float32(sampleRate),
    }

    output = append(output, stat)
  }

  return output
}

// When provided with parsed StatSamples this will loop through them and inject
// them into the pipe for processing and aggregation.
func bufferStatSamples(samples []*StatSample) {
  for _, s := range samples {
    StatPipe <- s
  }
}

// Setup the UDP stat collection listener. Make sure it's setup too clean up
// after itself when the server begins termination.
func startStatListener() {
  addr := net.UDPAddr {
    Port: statCollectionPort,
    IP: net.ParseIP(listenAddress),
  }

  log.Printf("listening on %s:%d", addr.IP, addr.Port)

  listener, err := net.ListenUDP("udp", &addr)
  defer listener.Close()

  if err != nil {
    log.Fatalf("Error: Failed too bind too address %s:%d: %s", addr.IP, addr.Port, err.Error())
  }

  message := make([]byte, MAX_UDP_PACKET_SIZE)

  for {
    byteCount, remoteAddress, err := listener.ReadFromUDP(message)

    if err != nil {
      log.Printf("Error: Unable too read UDP packet from %+v - %s", remoteAddress, err.Error())
      continue
    }

    go bufferStatSamples(parseMessages(message[:byteCount]))
  }
}

// Variables related too command line options and general configuration
var (
  debug bool
  flushInterval int64
  graphiteAddress string
  listenAddress string
  percentThreshold = Percentiles{}
  persistCountKeys int64
  receiveCounter string
  showVersion bool
  statCollectionPort int
)

// Central location for the configuration and definition of the various command
// line arguments.
func parseCLI() {
  flag.IntVar(&statCollectionPort, "port", 8125, "The UDP port too listen for metrics on.")

  flag.StringVar(&listenAddress, "address", "::", "The address too bind the server too.")
  flag.StringVar(&listenAddress, "a", "::", "The address too bind the server too (short hand).")

  flag.StringVar(&graphiteAddress, "graphite", "127.0.0.1:2003", "Graphite service address (or - to disable)")
  flag.Int64Var(&flushInterval, "flush-interval", 10, "Flush interval (seconds)")
  flag.BoolVar(&debug, "debug", false, "print statistics sent to graphite")
  flag.BoolVar(&showVersion, "version", false, "print version string")
  flag.Int64Var(&persistCountKeys, "persist-count-keys", 60, "number of flush-interval's to persist count keys")
  flag.StringVar(&receiveCounter, "receive-counter", "", "Metric name for total metrics recevied per interval")

  percentThreshold = Percentiles{}
  flag.Var(&percentThreshold, "percent-threshold", "Threshold percent (0-100, may be given multiple times)")

  flag.Parse()
}

func main() {
  parseCLI()

  if (showVersion) {
    fmt.Printf("Go Statsd v%s\n", VERSION)
    return
  }

  signalChannel = make(chan os.Signal, 1)
  signal.Notify(signalChannel, syscall.SIGTERM)
  signal.Notify(signalChannel, syscall.SIGINT)

  persistCountKeys = -1 * (persistCountKeys)

  go startStatListener()
  startCollector()
}

