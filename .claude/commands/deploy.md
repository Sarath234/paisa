---
description: Restart the local paisa server and paisa-agent (runs ./local_deploy.sh)
allowed-tools: Bash(./local_deploy.sh), Bash(tail:*)
---

## Deploy: restart the local paisa stack

Output of `./local_deploy.sh` (pkill reports "not running" when no process matched — that's fine):

!`./local_deploy.sh`

## Your task

Report deployment status based on the output above: confirm both processes have
PIDs. If either PID shows MISSING, check the tail of its log
(`~/Documents/paisa/log.txt` / `~/Documents/paisa/log-agent.txt`) and report
the failure reason.
