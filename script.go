package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
)

const script = `
# see: https://unix.stackexchange.com/questions/146756/forward-sigterm-to-child-in-bash
prep_term()
{
    unset term_child_pid
    unset term_kill_needed
    trap 'handle_term' TERM INT
}

handle_term()
{
    if [ "${term_child_pid}" ]; then
        kill -TERM "${term_child_pid}" 2>/dev/null
    else
        term_kill_needed="yes"
    fi
}

wait_term()
{
    if [ "${term_kill_needed}" ]; then
        kill -TERM "${term_child_pid}" 2>/dev/null
    fi
    wait ${term_child_pid} 2>/dev/null
    trap - TERM INT
    wait ${term_child_pid} 2>/dev/null
}

start_child()
{
	__add_child=1 "$exe" "$@"
}
`

var children []*exec.Cmd

type StartChildRequest struct {
	Out, Err uintptr
	Script   string
	Command  []string
}

type StartChildResponse struct {
	Pid      int
	Error    string
	ExitCode int
}

func initChildClient(port string, script bool, argv ...string) {
	var (
		servAddr = "localhost:" + port
		req      = StartChildRequest{Out: os.Stdout.Fd(), Err: os.Stderr.Fd()}
		buf      bytes.Buffer
		err      error
	)

	if script {
		if _, err = io.Copy(&buf, os.Stdin); err != nil {
			log.Fatalln("read stdin fail:", err)
			os.Exit(1)
		}
		req.Script = buf.String()
	} else {
		req.Command = argv
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
	if err != nil {
		log.Fatalln("ResolveTCPAddr failed:", err)
	}

	conn, err := net.DialTCP("tcp", nil, tcpAddr)
	if err != nil {
		log.Fatalln("Dial failed:", err)
	}
	defer conn.Close()

	var data []byte
	data, err = json.Marshal(req)
	if err != nil {
		log.Fatalln("Marshall request failed:", err)
	}
	conn.Write(data)

	var resp StartChildResponse
	if err = json.NewDecoder(conn).Decode(&resp); err != nil {
		log.Fatalln("Decode response failed:", err)
	}

	os.Stdout.WriteString(strconv.Itoa(resp.Pid)+"\n")
	if resp.Error != "" {
		os.Stderr.WriteString(resp.Error)
		os.Stderr.WriteString("\n")
	}
	os.Exit(resp.ExitCode)
}

func startChild(conn net.Conn) {
	var (
		err  error
		req  StartChildRequest
		resp StartChildResponse
	)

	defer conn.Close()

	if err = json.NewDecoder(conn).Decode(&req); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1

		b, _ := json.Marshal(resp)
		conn.Write(b)
		return
	}

	var opt CmdOpt

	if len(req.Command) > 0 {
		opt.Name, opt.Args = req.Command[0], req.Command[1:]
	} else {
		opt.Script = req.Script
	}
	
	opt.Out = req.Out
	opt.Err = req.Err

	child := opt.Create()
	if err = cmds.Start(child); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		b, _ := json.Marshal(resp)
		conn.Write(b)
		return
	}
	resp.Pid = child.Process.Pid
	b, _ := json.Marshal(resp)
	conn.Write(b)
}

func startChildServer() (port string) {
	// Listen for incoming connections on random port.
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("Error listening: %s\n", err)
	}

	go func() {
		// Close the listener when the application closes.
		defer l.Close()
		for {
			// Listen for an incoming connection.
			conn, err := l.Accept()
			if err != nil {
				log.Printf("START_CHILD_SERVER: accept fail: %s\n", err)
				return
			}
			go startChild(conn)
		}
	}()
	_, port, _ = net.SplitHostPort(l.Addr().String())
	return
}
