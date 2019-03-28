#! /bin/sh

sysctl -w net.core.rmem_max=67108864
sysctl -w net.ipv4.udp_mem="754848 1006464 1509696"
sysctl -w net.core.netdev_max_backlog=2000
sysctl -p
