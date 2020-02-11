#!/bin/bash

cd $(dirname $0)

f=${1:-Proto}

go-fuzz -func Fuzz$f -workdir ${f}_wd -bin fuzz-fuzz.zip
