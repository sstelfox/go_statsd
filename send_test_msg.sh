#!/bin/bash

echo "Sending a counter increment message..."
echo "testing.counter:$(($RANDOM % 10))|c" | nc -w 1 -u 127.0.0.1 8125
