#!/bin/bash

echo "Sending metrics..."

echo "testing.counter:$(($RANDOM % 10))|c" | nc -w 1 -u 127.0.0.1 8125

for var in 1 2 3 4 5 6 7 8 9 10; do
  echo "testing.timer:$(($RANDOM % 50))|ms" | nc -w 1 -u 127.0.0.1 8125
done

echo "testing.gauge:$(($RANDOM % 100))|g" | nc -w 1 -u 127.0.0.1 8125
