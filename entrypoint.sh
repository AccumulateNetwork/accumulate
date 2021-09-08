#!/usr/bin/env /bin/bash

#confd -onetime -sync-only -config-file "/home/app/confd/confd.toml"

#cat ~/boot_output.txt

{
  while true; do
    inotifywait -m ./values -e create -e modify -e delete -e move -r |
      while read path action file; do
        if [[ ${file} =~ .*\.yaml$ ]]; then
          echo "inotifywait: ${action} event on ${path}${file}"
          # confd -onetime -config-file "/home/app/confd/confd.toml"
          # cat ~/boot_output.txt
        fi
      done
  done
} &

exec ${@}
