#!/bin/bash

# AppSRE CD

set -exv

make -C $(dirname $0)/../ build-package-push
