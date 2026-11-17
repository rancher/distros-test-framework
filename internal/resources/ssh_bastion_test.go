package resources

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// startSSHServer serves SSH on 127.0.0.1 and hands each new channel to handle.
func startSSHServer(t *testing.T, handle func(ssh.NewChannel)) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() {
				_, chans, reqs, hsErr := ssh.NewServerConn(c, cfg)
				if hsErr != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					go handle(ch)
				}
			}()
		}
	}()

	return ln.Addr().String()
}

// execServer answers every exec request with reply on stdout and exit status 0.
func execServer(reply string) func(ssh.NewChannel) {
	return func(nc ssh.NewChannel) {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "session only")
			return
		}
		ch, reqs, err := nc.Accept()
		if err != nil {
			return
		}
		for req := range reqs {
			if req.Type != "exec" {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			_, _ = io.WriteString(ch, reply)
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			_ = ch.Close()

			return
		}
	}
}

// forwardingBastion records each direct-tcpip target and pipes it to realTarget.
func forwardingBastion(realTarget string, mu *sync.Mutex, seen *[]string) func(ssh.NewChannel) {
	return func(nc ssh.NewChannel) {
		var dst struct {
			Host       string
			Port       uint32
			OriginHost string
			OriginPort uint32
		}
		if nc.ChannelType() != "direct-tcpip" || ssh.Unmarshal(nc.ExtraData(), &dst) != nil {
			_ = nc.Reject(ssh.Prohibited, "direct-tcpip only")
			return
		}
		mu.Lock()
		*seen = append(*seen, net.JoinHostPort(dst.Host, "22"))
		mu.Unlock()
		up, err := net.Dial("tcp", realTarget)
		if err != nil {
			_ = nc.Reject(ssh.ConnectionFailed, err.Error())
			return
		}
		ch, reqs, err := nc.Accept()
		if err != nil {
			_ = up.Close()
			return
		}
		go ssh.DiscardRequests(reqs)
		go func() { _, _ = io.Copy(ch, up); _ = ch.CloseWrite() }()
		_, _ = io.Copy(up, ch)
		_ = up.Close()
	}
}

func useTestBastion(t *testing.T, ip, addr string) {
	t.Helper()
	sshBastion.Lock()
	sshBastion.ip, sshBastion.addr = ip, addr
	sshBastion.Unlock()
	t.Cleanup(func() {
		SetSSHBastion("")
		connPool.Lock()
		for k, c := range connPool.connClient {
			_ = c.Close()
			delete(connPool.connClient, k)
		}
		connPool.Unlock()
	})
}

// An IPv6 node unreachable from the runner is reached through the bastion's direct-tcpip channel.
func TestRunCommandOnNodeTunnelsIPv6ThroughBastion(t *testing.T) {
	t.Setenv("PROVISIONER_MODULE", "legacy")
	t.Setenv("access_key", writeTestKey(t))
	t.Setenv("aws_user", "test")

	target := startSSHServer(t, execServer("from-node"))
	var mu sync.Mutex
	var seen []string
	bastion := startSSHServer(t, forwardingBastion(target, &mu, &seen))
	useTestBastion(t, "127.0.0.1", bastion)

	// 2001:db8::/32 is documentation-only, so a direct dial could never answer.
	out, err := RunCommandOnNode("hostname", "2001:db8::10")
	if err != nil {
		t.Fatalf("RunCommandOnNode via bastion: %v", err)
	}
	if out != "from-node" {
		t.Errorf("stdout = %q, want from-node", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "[2001:db8::10]:22" {
		t.Errorf("bastion forwarded %v, want [[2001:db8::10]:22]", seen)
	}
}

func TestBastionForOnlyRoutesIPv6Targets(t *testing.T) {
	SetSSHBastion("203.0.113.5")
	t.Cleanup(func() { SetSSHBastion("") })
	cases := map[string]string{
		"[2001:db8::10]:22": "203.0.113.5:22",
		"198.51.100.7:22":   "",
		"203.0.113.5:22":    "",
		"not-a-host-port":   "",
	}
	for host, want := range cases {
		if got := bastionFor(host); got != want {
			t.Errorf("bastionFor(%q) = %q, want %q", host, got, want)
		}
	}
	SetSSHBastion("")
	if got := bastionFor("[2001:db8::10]:22"); got != "" {
		t.Errorf("no bastion registered must dial directly, got %q", got)
	}
}

// A node that never speaks SSH behind the bastion must not outlive ctx: the tunnel has no deadline.
func TestRunCommandOnNodeContextBoundsStalledNodeBehindBastion(t *testing.T) {
	t.Setenv("PROVISIONER_MODULE", "legacy")
	t.Setenv("access_key", writeTestKey(t))
	t.Setenv("aws_user", "test")

	stalled, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stalled.Close()
	var mu sync.Mutex
	var seen []string
	bastion := startSSHServer(t, forwardingBastion(stalled.Addr().String(), &mu, &seen))
	useTestBastion(t, "127.0.0.1", bastion)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = RunCommandOnNodeContext(ctx, "hostname", "2001:db8::10")
	if err == nil {
		t.Fatal("expected an error from a stalled handshake behind the bastion")
	}
	mu.Lock()
	forwarded := len(seen)
	mu.Unlock()
	if forwarded != 1 {
		t.Fatalf("the dial must reach the bastion tunnel, got %d forwards (err: %v)", forwarded, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("dial outlived its context: %s", elapsed)
	}
}

// RunCommandOnNodeContext must bracket IPv6 literals; a bare ip+":22" is not a valid address.
func TestRunCommandOnNodeContextTunnelsIPv6ThroughBastion(t *testing.T) {
	t.Setenv("PROVISIONER_MODULE", "legacy")
	t.Setenv("access_key", writeTestKey(t))
	t.Setenv("aws_user", "test")

	target := startSSHServer(t, execServer("ctx-node"))
	var mu sync.Mutex
	var seen []string
	useTestBastion(t, "127.0.0.1", startSSHServer(t, forwardingBastion(target, &mu, &seen)))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := RunCommandOnNodeContext(ctx, "hostname", "2001:db8::10")
	if err != nil || out != "ctx-node" {
		t.Fatalf("RunCommandOnNodeContext via bastion = %q, %v", out, err)
	}
}
