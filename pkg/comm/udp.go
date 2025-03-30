package comm

import (
	"log"
	"net"

	"github.com/myl7/zyzzyva/pkg/conf"
	"github.com/myl7/zyzzyva/pkg/utils"
)

func UdpSend(b []byte, tid int) {
	addr, err := net.ResolveUDPAddr("udp", conf.GetReqAddr(tid))
	if err != nil {
		panic(err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	err = conn.SetWriteBuffer(1 * 1024 * 1024)
	if err != nil {
		panic(err)
	}

	_, err = conn.Write(b)
	if err != nil {
		panic(err)
	}
}

func UdpMulticast(b []byte) {
	conn, err := net.Dial("udp", conf.UdpMulticastAddr)
	if err != nil {
		panic(err)
	}

	_, err = conn.Write(b)
	if err != nil {
		panic(err)
	}
}

func UdpSendObj(obj interface{}, tid int) {
	UdpSend(utils.Ser(obj), tid)
}

func UdpMulticastObj(obj interface{}) {
	UdpMulticast(utils.Ser(obj))
}

func UdpListen(addr string, handle func([]byte)) {
	l, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}

	buf := make([]byte, conf.UdpBufSize)

	for {
		n, _, err := l.ReadFrom(buf)
		if err != nil {
			panic(err)
		}

		b := buf[:n]
		go handle(b)
	}
}

// func UdpListenMulticast(handle func([]byte)) {
// 	var ifi *net.Interface
// 	for i := range conf.UdpMulticastInterfaces {
// 		var err error
// 		ifi, err = net.InterfaceByName(conf.UdpMulticastInterfaces[i])
// 		if err != nil {
// 			if conf.UdpMulticastInterfaces[i] == "" {
// 				ifi = nil
// 				break
// 			} else if i == len(conf.UdpMulticastInterfaces)-1 {
// 				panic(err)
// 			}
// 		} else {
// 			break
// 		}
// 	}

// 	addr, err := net.ResolveUDPAddr("udp", conf.UdpMulticastAddr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	l, err := net.ListenMulticastUDP("udp", ifi, addr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	err = l.SetReadBuffer(1 * 1024 * 1024)
// 	if err != nil {
// 		panic(err)
// 	}

// 	buf := make([]byte, 1*1024*1024)

// 	for {
// 		n, _, err := l.ReadFrom(buf)
// 		if err != nil {
// 			panic(err)
// 		}

//			b := buf[:n]
//			go handle(b)
//		}
//	}
func UdpListenMulticast(handle func([]byte)) {
	var ifi *net.Interface

	// Try to find a valid interface
	if len(conf.UdpMulticastInterfaces) > 0 && conf.UdpMulticastInterfaces[0] != "" {
		for _, ifName := range conf.UdpMulticastInterfaces {
			if ifName == "" {
				continue
			}

			var err error
			ifi, err = net.InterfaceByName(ifName)
			if err == nil {
				break
			}
			log.Printf("Warning: Couldn't find multicast interface %s: %v", ifName, err)
		}
	}

	// If we couldn't find any of the specified interfaces, use nil (default)
	if ifi == nil && len(conf.UdpMulticastInterfaces) > 0 && conf.UdpMulticastInterfaces[0] != "" {
		log.Println("Warning: None of the specified multicast interfaces found, using default")
	}

	addr, err := net.ResolveUDPAddr("udp", conf.UdpMulticastAddr)
	if err != nil {
		panic(err)
	}

	l, err := net.ListenMulticastUDP("udp", ifi, addr)
	if err != nil {
		panic(err)
	}

	// Set read buffer size - no type assertion needed since l is already *net.UDPConn
	err = l.SetReadBuffer(conf.UdpBufSize)
	if err != nil {
		log.Printf("Warning: Failed to set UDP read buffer size: %v", err)
	}

	buf := make([]byte, conf.UdpBufSize)

	for {
		n, _, err := l.ReadFrom(buf)
		if err != nil {
			panic(err)
		}

		b := buf[:n]
		go handle(b)
	}
}
