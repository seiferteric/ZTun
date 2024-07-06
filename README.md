# ZTun Compressed Tunnel

ZTun is a simple way to set up a TUN interface between systems that transparently compresses data over said interface. The inspiration for this is that some (older) protocols don't nativaly support any compression and yet can use a lot of bandwidth. In order to make it more efficient, you can run these protocols over a ZTun and get compression.

ZTun uses lz4 compression which is fast and has reasonably good compression. You can run a ZTun server on one machine and have multiple clients connect to it.

## Usage

### Help:

    $ ./ztun -h
    Usage of ./ztun:
    -mtu uint
            Interface MTU. Larger MTU can help with compression. (default 9000)
    -port uint
            Port to use. (default 2024)
    -server string
            ztun Server to connect to.
    -subnet string
            Subnet that will be used for network. (default "172.168.13.1/24")

### Server:

With defaults, creates a /24 and assignes the first IP in range to the server:

    $ sudo ./ztun 
    2024/07/05 16:29:18 Serving 192.168.1.3:57742

    $ ip a s ztun0
    138: ztun0: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP> mtu 9000 qdisc fq_codel state UNKNOWN group default qlen 500
        link/none 
        inet 172.168.13.1/24 brd 172.168.13.255 scope global ztun0
        valid_lft forever preferred_lft forever
        inet6 fe80::8575:451f:c78:5247/64 scope link stable-privacy proto kernel_ll 
        valid_lft forever preferred_lft forever


### Client:

The client connects to the server and gets assigned an IP from the server:

    $ sudo ./ztun --server=192.168.1.5 
    2024/07/05 23:29:18 Serving 192.168.1.5:2024

    $ ip a s ztun0
    133: ztun0: <POINTOPOINT,MULTICAST,NOARP,UP,LOWER_UP> mtu 9000 qdisc fq_codel state UNKNOWN group default qlen 500
        link/none 
        inet 172.168.13.2/24 brd 172.168.13.255 scope global ztun0
        valid_lft forever preferred_lft forever
        inet6 fe80::24c4:e774:4c30:a65/64 scope link stable-privacy 
        valid_lft forever preferred_lft forever


## Testing Results

On a 1Gbps LAN connection between two machines with iperf3 with repeating payload (helps with compression) we see significant speedup of ~4.3Gbps:

    $ iperf3 -c 172.168.13.1 -p 5201 --repeating-payload
    Connecting to host 172.168.13.1, port 5201
    [  5] local 172.168.13.2 port 36154 connected to 172.168.13.1 port 5201
    [ ID] Interval           Transfer     Bitrate         Retr  Cwnd
    [  5]   0.00-1.00   sec   521 MBytes  4.37 Gbits/sec    0   3.82 MBytes
    [  5]   1.00-2.00   sec   486 MBytes  4.08 Gbits/sec    0   3.82 MBytes
    [  5]   2.00-3.00   sec   499 MBytes  4.18 Gbits/sec    0   3.82 MBytes
    [  5]   3.00-4.00   sec   508 MBytes  4.26 Gbits/sec    0   3.82 MBytes
    [  5]   4.00-5.00   sec   526 MBytes  4.41 Gbits/sec    0   3.82 MBytes
    [  5]   5.00-6.00   sec   506 MBytes  4.25 Gbits/sec    0   3.82 MBytes
    [  5]   6.00-7.00   sec   500 MBytes  4.19 Gbits/sec    0   3.82 MBytes
    [  5]   7.00-8.00   sec   529 MBytes  4.43 Gbits/sec    0   3.82 MBytes
    [  5]   8.00-9.00   sec   512 MBytes  4.30 Gbits/sec    0   3.82 MBytes
    [  5]   9.00-10.00  sec   550 MBytes  4.62 Gbits/sec    0   3.82 MBytes
    - - - - - - - - - - - - - - - - - - - - - - - - -
    [ ID] Interval           Transfer     Bitrate         Retr
    [  5]   0.00-10.00  sec  5.02 GBytes  4.31 Gbits/sec    0             sender
    [  5]   0.00-10.01  sec  5.02 GBytes  4.31 Gbits/sec                  receiver
    
    iperf Done.


With randomized payload (bad for compression) it is slightly slower than native:

    $ iperf3 -c 172.168.13.1 -p 5201 
    Connecting to host 172.168.13.1, port 5201
    [  5] local 172.168.13.2 port 60858 connected to 172.168.13.1 port 5201
    [ ID] Interval           Transfer     Bitrate         Retr  Cwnd
    [  5]   0.00-1.00   sec  91.2 MBytes   765 Mbits/sec    0   3.99 MBytes       
    [  5]   1.00-2.00   sec  88.8 MBytes   745 Mbits/sec    0   3.99 MBytes       
    [  5]   2.00-3.00   sec  88.8 MBytes   744 Mbits/sec    0   3.99 MBytes       
    [  5]   3.00-4.00   sec  88.8 MBytes   744 Mbits/sec    0   3.99 MBytes       
    [  5]   4.00-5.00   sec  90.0 MBytes   755 Mbits/sec    0   3.99 MBytes       
    [  5]   5.00-6.00   sec  88.8 MBytes   744 Mbits/sec    0   3.99 MBytes       
    [  5]   6.00-7.00   sec  88.8 MBytes   745 Mbits/sec    0   3.99 MBytes       
    [  5]   7.00-8.00   sec  88.8 MBytes   744 Mbits/sec    0   3.99 MBytes       
    [  5]   8.00-9.00   sec  88.8 MBytes   744 Mbits/sec    0   3.99 MBytes       
    [  5]   9.00-10.00  sec  90.0 MBytes   755 Mbits/sec    0   3.99 MBytes       
    - - - - - - - - - - - - - - - - - - - - - - - - -
    [ ID] Interval           Transfer     Bitrate         Retr
    [  5]   0.00-10.00  sec   892 MBytes   749 Mbits/sec    0             sender
    [  5]   0.00-10.03  sec   892 MBytes   746 Mbits/sec                  receiver

    iperf Done.


Native:

    $ iperf3 -c 192.168.1.5 -p 5201 
    Connecting to host 192.168.1.5, port 5201
    [  5] local 192.168.1.3 port 56220 connected to 192.168.1.5 port 5201
    [ ID] Interval           Transfer     Bitrate         Retr  Cwnd
    [  5]   0.00-1.00   sec  90.0 MBytes   754 Mbits/sec  1644   49.5 KBytes       
    [  5]   1.00-2.00   sec  91.6 MBytes   769 Mbits/sec  1457   65.0 KBytes       
    [  5]   2.00-3.00   sec  91.7 MBytes   770 Mbits/sec  1426   82.0 KBytes       
    [  5]   3.00-4.00   sec  90.9 MBytes   763 Mbits/sec  1569   59.4 KBytes       
    [  5]   4.00-5.00   sec  90.8 MBytes   762 Mbits/sec  1590   49.5 KBytes       
    [  5]   5.00-6.00   sec  90.7 MBytes   761 Mbits/sec  1559   69.3 KBytes       
    [  5]   6.00-7.00   sec  89.7 MBytes   753 Mbits/sec  1441   69.3 KBytes       
    [  5]   7.00-8.00   sec  91.3 MBytes   766 Mbits/sec  1327   89.1 KBytes       
    [  5]   8.00-9.00   sec  90.8 MBytes   762 Mbits/sec  1556   52.3 KBytes       
    [  5]   9.00-10.00  sec  89.8 MBytes   754 Mbits/sec  1561   69.3 KBytes       
    - - - - - - - - - - - - - - - - - - - - - - - - -
    [ ID] Interval           Transfer     Bitrate         Retr
    [  5]   0.00-10.00  sec   907 MBytes   761 Mbits/sec  15130             sender
    [  5]   0.00-10.00  sec   906 MBytes   760 Mbits/sec                  receiver

    iperf Done.



