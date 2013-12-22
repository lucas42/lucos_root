#!/bin/sh
cd $(dirname $0)
mkdir -p bin
javac -cp .:../lib/java/* -d bin *.java -Xlint:unchecked
