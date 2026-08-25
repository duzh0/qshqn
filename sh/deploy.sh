#!/usr/bin/env bash
rsync -avzP -e "ssh -p 2022" out/qshqn-vps root@10.0.0.1:~/bots/qshqn/qshqn2
