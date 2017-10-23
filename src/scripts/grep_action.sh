#!/usr/bin/env bash

if [ "$#" -ne 1 ]; then
    echo "Please input filepath"
    exit -1
fi

file_path=$1

grep "操作" $file_path