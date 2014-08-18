#!/bin/bash

echo "Starting up TCP server on port 2003 to collect statistics sent too graphite..."

nc -lk 2003

echo "Closing..."
