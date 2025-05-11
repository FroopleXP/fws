package main

import (
    "log"
    "net"
    "io"
)

const sockAddr string = ":3000"

func handle(c net.Conn) {
    defer c.Close()

    var buf []byte = make([]byte, 1024)

    for {
        n, err := c.Read(buf)
        if n > 0 {
            log.Printf("from client: %v\n", string(buf[:n]))
        }

        if err != nil {
            if err == io.EOF {
                return
            }
            log.Printf("failed to read from client: %v\n", err)
        }
    }
}

func main() {
    log.Printf("starting socket server on %s", sockAddr)
    l, err := net.Listen("tcp", sockAddr)
    if err != nil {
        log.Fatalf("failed to start socket server: %v\n", err)
    }

    for {
        c, err := l.Accept()
        if err != nil {
            log.Printf("failed to accept incoming connection: %v\n", err)
            continue
        }

        if err := upgrade(c); err != nil {
            log.Printf("failed to upgrade client: %v\n", err)
        }

        go handle(c)
    }
}
