package main

import (
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestDescribeListenConflictNamesTheOccupant(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	text := describeListenConflict("127.0.0.1:" + port)
	if !strings.Contains(text, "端口 "+port+" 已被占用") {
		t.Fatalf("missing port notice: %s", text)
	}
	if !strings.Contains(text, "PID="+strconv.Itoa(os.Getpid())) {
		t.Fatalf("missing occupant pid: %s", text)
	}
	if !strings.Contains(text, "其它站点") {
		t.Fatalf("missing handling advice: %s", text)
	}
}

func TestDescribeListenConflictWhenPortIsFree(t *testing.T) {
	text := describeListenConflict("127.0.0.1:1")
	if !strings.Contains(text, "已被占用") || !strings.Contains(text, "看不到占用进程") {
		t.Fatalf("unexpected free-port text: %s", text)
	}
}
