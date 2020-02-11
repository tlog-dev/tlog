#!/bin/bash

cd $(dirname $0)

go-fuzz-build github.com/nikandfor/tlog/fuzz
