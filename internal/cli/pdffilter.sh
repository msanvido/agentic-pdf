#!/bin/sh
# agentic-pdf pass-through filter: forwards the PDF spool untouched
# to the backend. CUPS filter contract: job-id user title copies options [file]
[ -n "$6" ] && exec cat "$6"
exec cat
