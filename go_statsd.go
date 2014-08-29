
package main

import (
  "bytes"
  "flag"
  "fmt"
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
  VERSION                 = "0.6.0"
  MAX_UNPROCESSED_PACKETS = 2048
  MAX_UDP_PACKET_SIZE     = 784
)

type StatSample struct {
  Bucket     string
  Value      interface{}
  Modifier   string
  SampleRate float32
}

// This data type is for metric aggregation, the functions implement the
// sort.Interface.
type Int64Slice []int64
func (s Int64Slice) Len() int           { return len(s) }
func (s Int64Slice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s Int64Slice) Less(i, j int) bool { return s[i] < s[j] }

// Implementation of the flag.Value interface for collecting several comma
// separated integer values into an array.
type Percentiles []int
func (i *Percentiles) String() string { return fmt.Sprintf("%v", *i) }
func (i *Percentiles) Set(value string) error {
  // Reset any existing values before setting up the requested percentiles
  *i = make(Percentiles, 0)

  for _, element := range strings.Split(value, ",") {
    value, err := strconv.Atoi(element)

    if err != nil {
      return err
    }

    *i = append(*i, value)
  }

  return nil
}

// A simple structure for tracking whether or not we've seen a value before or
// not.
type StringSet map[string]bool
func (set StringSet) Add(s string) { set[s] = true }

// Communication channels and global metric variables.
var (
  StatPipe      = make(chan *StatSample, MAX_UNPROCESSED_PACKETS)
  signalChannel = make(chan os.Signal, 1)

  counters      = make(map[string]int64)
  gauges        = make(map[string]int64)
  sets          = make(map[string]StringSet)
  timers        = make(map[string]Int64Slice)
)

// This handles the processing of individual already parsed StatSample
// messages into the aggregate counts as well as periodically triggering the
// flush too graphite.
func startCollector() {
  // Limit the amount of time we attempt too submit information to the backend
  // too the flushInterval so data is always sent in the correct order and we
  // don't have several hanging open connections.
  period := time.Duration(flushInterval) * time.Second

  // The timer that'll trigger our submission of aggregate data
  publishTimer := time.NewTicker(period)

  // Main processing loop, the other threads are effectively evented and only
  // push data too this loop.
  for {
    select {
      // Handle various signals passed too us by the operating system, for the
      // time being we handle all of the signals we're watching as a
      // notification too exit.
      case sig := <-signalChannel:
        fmt.Printf("!! Caught signal %d... shutting down\n", sig)
        publishAggregates(time.Now().Add(period))
        return
      // Whenever our timer fires it's time too dump our stats too graphite.
      case <-publishTimer.C:
        publishAggregates(time.Now().Add(period))
      // If we haven't hit on the other two process any pending stats in our
      // collection pipe.
      case s := <-StatPipe:
        // If we're tracking total received stats, initialize the counter if
        // necessary and increment it for this interval.
        if (receiveCounter != "") {
          _, ok := counters[receiveCounter]
          if (!ok) { counters[receiveCounter] = 0 }
          counters[receiveCounter] += 1
        }

        if s.Modifier == "ms" {
          // Handle timers
          _, ok := timers[s.Bucket]
          if !ok { timers[s.Bucket] = make(Int64Slice, 0) }
          timers[s.Bucket] = append(timers[s.Bucket], s.Value.(int64))
        } else if s.Modifier == "g" {
          // Handle gauges
          // TODO: Handle modifiers +/-
          gauges[s.Bucket] = s.Value.(int64)
        } else if s.Modifier == "s" {
          _, ok := sets[s.Bucket]
          if !ok { sets[s.Bucket] = make(StringSet, 0) }
          sets[s.Bucket].Add(s.Value.(string))
        } else {
          // Handle counter types
          _, ok := counters[s.Bucket]
          if (!ok) { counters[s.Bucket] = 0 }
          counters[s.Bucket] += int64(float64(s.Value.(int64)) * float64(1 / s.SampleRate))
        }
    }
  }
}

// If there are any collected metrics since the last time this was called,
// format them and attempt to send them too the configured graphite server.
func publishAggregates(deadline time.Time) {
  var buffer bytes.Buffer
  var num int64

  now := time.Now().Unix()

  num += processCounters(&buffer, now)
  num += processGauges(&buffer, now)
  num += processSets(&buffer, now)
  num += processTimers(&buffer, now, percentileThresholds)

  if num == 0 {
    return
  }

  client, err := net.Dial("tcp", graphiteAddress)
  if err != nil {
    fmt.Printf("Error: Unable too publish counts - Dialing %s failed - %s\n", graphiteAddress, err.Error())
    return
  }
  defer client.Close()

  err = client.SetDeadline(deadline)
  if err != nil {
    fmt.Printf("Error: Could not set deadline on connection to %s - %s\n", graphiteAddress, err.Error())
    return
  }

  _, err = client.Write(buffer.Bytes())
  if err != nil {
    fmt.Printf("Error: Failed to write stats to %s - %s\n", graphiteAddress, err.Error())
    return
  }

  fmt.Printf("Sent %d stats to %s\n", num, graphiteAddress)

  return
}

// Process counters and rates of incoming counters, then add the results too
// the buffer. This will return the total number of individual counter metrics
// added too the buffer.
func processCounters(buffer *bytes.Buffer, now int64) int64 {
  var num int64

  for metric, count := range counters {
    rate := (float64(count) / float64(flushInterval))

    fmt.Fprintf(buffer, "stats.counters.%s.count %d %d\n", metric, count, now)
    fmt.Fprintf(buffer, "stats.counters.%s.rate %f %d\n", metric, rate, now)

    delete(counters, metric)
    num += 2
  }

  return num
}

// TODO: Track whether or not gauges are 'dirty' (value has changed since the
// last time we sent metrics) instead of just deleting the value. This will
// allow us too support 'modifiers' on gauges.
func processGauges(buffer *bytes.Buffer, now int64) int64 {
  var num int64

  for metric, value := range gauges {
    fmt.Fprintf(buffer, "stats.gauges.%s %d %d\n", metric, value, now)
    delete(gauges, metric)
    num++
  }

  return num
}

// Go through and handle our sets data objects, collecting and resetting the
// counts of the unique values we've seen.
func processSets(buffer *bytes.Buffer, now int64) int64 {
  var num int64

  // TODO: If a set is empty, we should send a single count of 0 then delete
  // the set entirely
  for metric, set := range sets {
    fmt.Fprintf(buffer, "stats.sets.%s.count %d %d\n", metric, len(set), now)
    delete(sets, metric)
    num++
  }

  return num
}

func processTimers(buffer *bytes.Buffer, now int64, pctls Percentiles) int64 {
  var num int64

  for key, times := range timers {
    if len(times) > 0 {
      sort.Sort(times)

      min := times[0]
      max := times[len(times)-1]
      maxAtThreshold := max
      count := len(times)

      sum := int64(0)
      for _, value := range times {
        sum += value
      }
      mean := float64(sum) / float64(len(times))

      for _, pct := range pctls {
        var tmpl string
        var abs int
        if pct >= 0 {
          abs = pct
          tmpl = "stats.timers.%s.upper_%d %d %d\n"
        } else {
          abs = 100 + pct
          tmpl = "stats.timers.%s.lower_%d %d %d\n"
        }

        if len(times) > 1 {
          // poor man's math.Round(x):
          // math.Floor(x + 0.5)
          indexOfPerc := int(math.Floor(((float64(abs) / 100.0) * float64(count)) + 0.5))
          if pct >= 0 {
            indexOfPerc -= 1 // index offset=0
          }
          maxAtThreshold = times[indexOfPerc]
        }

        if pct < 0 {
          pct = pct * -1
        }

        fmt.Fprintf(buffer, tmpl, key, pct, maxAtThreshold, now)

        num++
      }

      timers[key] = make(Int64Slice, 0)

      fmt.Fprintf(buffer, "stats.timers.%s.mean %f %d\n", key, mean, now)
      fmt.Fprintf(buffer, "stats.timers.%s.upper %d %d\n", key, max, now)
      fmt.Fprintf(buffer, "stats.timers.%s.lower %d %d\n", key, min, now)
      fmt.Fprintf(buffer, "stats.timers.%s.count %d %d\n", key, count, now)

      num += 4
    }
  }

  return num
}

// TODO 'Gauges' can have a + or - sign at the beginning of their value
// indicating that the current value should be modified rather than set too a
// static value.
var packetRegexp = regexp.MustCompile("^([^:]+):(.+)\\|(c|g|s|ms)(\\|@([0-9\\.]+))?\n?$")

// TODO: Handle gauge modifiers
func parseMessages(data []byte) []*StatSample {
  var output []*StatSample

  for _, line := range bytes.Split(data, []byte("\n")) {
    if len(line) == 0 {
      continue
    }

    item := packetRegexp.FindSubmatch(line)
    if len(item) == 0 {
      fmt.Printf("Received invalid / unknown stat: %s\n", line)
      continue
    }

    var err error
    var value interface{}

    metric_type := string(item[3])

    if metric_type == "s" {
      value = string(item[2])
    } else {
      value, err = strconv.ParseInt(string(item[2]), 10, 64)
      if err != nil {
        fmt.Printf("Error: failed to ParseInt %s - %s\n", item[2], err)
        continue
      }
    }

    sampleRate, err := strconv.ParseFloat(string(item[5]), 32)
    if err != nil {
      sampleRate = 1
    }

    stat := &StatSample{
      Bucket:     string(item[1]),
      Value:      value,
      Modifier:   metric_type,
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

  fmt.Printf("Listening on %s:%d\n", addr.IP, addr.Port)

  listener, err := net.ListenUDP("udp", &addr)
  defer listener.Close()

  if err != nil {
    fmt.Printf("Error: Failed too bind too address %s:%d: %s\n", addr.IP, addr.Port, err.Error())
    panic(err)
  }

  message := make([]byte, MAX_UDP_PACKET_SIZE)

  for {
    byteCount, remoteAddress, err := listener.ReadFromUDP(message)

    if err != nil {
      fmt.Printf("Error: Unable too read UDP packet from %+v - %s\n", remoteAddress, err.Error())
      continue
    }

    go bufferStatSamples(parseMessages(message[:byteCount]))
  }
}

// Variables related too command line options and general configuration
var (
  flushInterval int64
  graphiteAddress string
  listenAddress string
  percentileThresholds = Percentiles{}
  receiveCounter string
  showVersion bool
  statCollectionPort int
)

// Central location for the configuration and definition of the various command
// line arguments.
func parseCLI() {
  flag.IntVar(&statCollectionPort, "port", 8125, "The UDP port too listen for metrics on.")
  flag.StringVar(&listenAddress, "address", "0.0.0.0", "The address too bind the server too.")
  flag.StringVar(&graphiteAddress, "graphite", "127.0.0.1:2003", "Graphite service address (or - to disable)")
  flag.Int64Var(&flushInterval, "interval", 10, "Flush interval (seconds)")
  flag.BoolVar(&showVersion, "version", false, "Print version string and quit.")
  flag.StringVar(&receiveCounter, "receive-counter", "statsd.count", "Metric name for total metrics recevied per interval")

  percentileThresholds = Percentiles{50,90}
  flag.Var(&percentileThresholds, "percentiles", "Percentile limits calculated on timers. Multiple values can be passed as a comma separated list. If multiple instances are provided only the last one will be used.")

  flag.Parse()
}

func main() {
  parseCLI()

  if (showVersion) {
    fmt.Printf("Go Statsd v%s\n", VERSION)
    return
  }

  signal.Notify(signalChannel, syscall.SIGTERM)
  signal.Notify(signalChannel, syscall.SIGINT)

  go startStatListener()
  startCollector()
}

