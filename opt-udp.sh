#! /bin/sh

sysctl -w net.core.rmem_max=67108864
sysctl -w net.ipv4.udp_mem="754848 1006464 1509696"
sysctl -w net.core.netdev_max_backlog=2000
sysctl -p

# increase send udp buffer
#sysctl -w net.unix.max_dgram_qlen=1024

# increase send if tx qlen, 10k for no lost send or 2k with 500ns send interval
#ifconfig p1p1 txqueuelen 10000
