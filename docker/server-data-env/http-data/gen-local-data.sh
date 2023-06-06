#!/usr/bin/env bash

echo [Generating 1MB target file...]
cat /dev/random | dd count=1048576 iflag=count_bytes > target_1M.dat

echo [Generating 10MB target file...]
cat /dev/random | dd count=10485760 iflag=count_bytes > target_10M.dat

echo [Generating 100MB target file...]
cat /dev/random | dd count=104857600 iflag=count_bytes > target_100M.dat
