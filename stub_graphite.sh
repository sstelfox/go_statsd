#!/bin/bash

echo "Starting up TCP server on port 2003 to collect statistics sent too graphite..."

nc -vlk 2003

