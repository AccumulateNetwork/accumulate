#!/bin/bash

source ~/node.env
screen -d -m -S accumulated bash -c "accumulated run -w ~/.accumulate/bvc${BVC} -n ${NODE} 2>&1 | tee ~/acc-$(date +%Y%m%d%H%M%S).log"