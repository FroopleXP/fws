package main

import (
    "net"
    "io"
    "fmt"
    "strings"
    "crypto/sha1"
    "encoding/base64"
    "bufio"
)

const handshakeGuid string = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func sendHttpResponse(w io.Writer, code int) error {
    res := ""
    res += fmt.Sprintf("HTTP/1.1 %d Bad Request\r\n", code)
    res += fmt.Sprintf("\r\n")
    if _, err := w.Write([]byte(res)); err != nil {
        return err
    }
    return nil
}

func generateAcceptKey(key string) (string, error) {
    combinedKey := key + handshakeGuid
    hasher := sha1.New()
    _, err := hasher.Write([]byte(combinedKey))
    if err != nil {
        return "", err
    }
    return base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}

func upgrade(c net.Conn) error {
    reqHeaders := make(map[string]string, 0)

    scanner := bufio.NewScanner(c)
    for scanner.Scan() {
        entry := scanner.Text()
        if entry == "" { // We're at the end of the request
            break
        }
        splits := strings.Split(entry, ":")
        if len(splits) < 2 { // We need at least 2 parts to form the key/val pair
            continue
        }
        reqHeaders[strings.TrimSpace(splits[0])] = strings.TrimSpace(splits[1])
    }

    if err := scanner.Err(); err != nil {
        if err == io.EOF {
            return fmt.Errorf("client %s disconnected\n", c.RemoteAddr())
        }
        if err := sendHttpResponse(c, 500); err != nil {
            return err
        }
    }

    secWebSocketKey, ok := reqHeaders["Sec-WebSocket-Key"]
    if !ok {
        if err := sendHttpResponse(c, 400); err != nil {
            return err
        }
        return fmt.Errorf("handshake invalid, could not find 'Sec-WebSocket-Key' in client request")
    }

    acceptKey, err := generateAcceptKey(secWebSocketKey)
    if err != nil {
        if err := sendHttpResponse(c, 500); err != nil {
            return err
        }
        return err
    }

    handshakeRes := ""
    handshakeRes += fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n")
    handshakeRes += fmt.Sprintf("Upgrade: websocket\r\n")
    handshakeRes += fmt.Sprintf("Connection: Upgrade\r\n")
    handshakeRes += fmt.Sprintf("Sec-WebSocket-Accept: %s\r\n", acceptKey)
    handshakeRes += fmt.Sprintf("Sec-WebSocket-Version: %d\r\n", 13)
    handshakeRes += fmt.Sprintf("\r\n")

    if _, err := c.Write([]byte(handshakeRes)); err != nil {
        return err
    }

    return nil
}
