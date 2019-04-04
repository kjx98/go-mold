#! /bin/sh

# for moldUDP64 test
firewall-cmd --permanent --zone=public --add-port=5858/udp
firewall-cmd --permanent --zone=public --add-port=5859/udp
firewall-cmd --reload
