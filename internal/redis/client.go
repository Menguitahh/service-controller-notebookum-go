package redis

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal Redis client for GET, SETEX, and EXISTS using raw RESP protocol.
// It opens a new TCP connection per call — suitable for low-frequency cache lookups.
type Client struct {
	addr     string
	password string
	timeout  time.Duration
}

func New(host, port, password string) *Client {
	return &Client{
		addr:     host + ":" + port,
		password: password,
		timeout:  3 * time.Second,
	}
}

// Get returns (value, true, nil) when key exists, ("", false, nil) when it does not.
func (c *Client) Get(key string) (string, bool, error) {
	conn, err := c.dial()
	if err != nil {
		return "", false, err
	}
	defer conn.Close()

	if err := write(conn, "GET", key); err != nil {
		return "", false, err
	}
	return readBulk(bufio.NewReader(conn))
}

// SetEX stores key=value with an expiry of ttlSeconds.
func (c *Client) SetEX(key, value string, ttlSeconds int) error {
	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := write(conn, "SET", key, value, "EX", strconv.Itoa(ttlSeconds)); err != nil {
		return err
	}
	_, err = readLine(bufio.NewReader(conn))
	return err
}

// Exists reports whether key is present in Redis.
func (c *Client) Exists(key string) (bool, error) {
	conn, err := c.dial()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	if err := write(conn, "EXISTS", key); err != nil {
		return false, err
	}
	line, err := readLine(bufio.NewReader(conn))
	if err != nil {
		return false, err
	}
	if len(line) == 0 || line[0] != ':' {
		return false, fmt.Errorf("redis: expected integer reply, got %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	return n > 0, err
}

// dial opens an authenticated TCP connection.
func (c *Client) dial() (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(c.timeout)) //nolint:errcheck

	if c.password != "" {
		if err := write(conn, "AUTH", c.password); err != nil {
			conn.Close()
			return nil, err
		}
		if _, err := readLine(bufio.NewReader(conn)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// write encodes args as a RESP array and sends it to conn.
func write(conn net.Conn, args ...string) error {
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(a), a)
	}
	_, err := io.WriteString(conn, sb.String())
	return err
}

// readLine reads one RESP line and strips CRLF. Redis error replies become Go errors.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) > 0 && line[0] == '-' {
		return "", fmt.Errorf("redis: %s", line[1:])
	}
	return line, nil
}

// readBulk reads a RESP bulk-string reply.
// Returns ("", false, nil) for a nil reply ($-1).
func readBulk(r *bufio.Reader) (string, bool, error) {
	line, err := readLine(r)
	if err != nil {
		return "", false, err
	}
	if len(line) == 0 || line[0] != '$' {
		return "", false, fmt.Errorf("redis: expected bulk string, got %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil {
		return "", false, err
	}
	if n == -1 {
		return "", false, nil // key does not exist
	}
	buf := make([]byte, n+2) // +2 for trailing \r\n
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", false, err
	}
	return string(buf[:n]), true, nil
}
