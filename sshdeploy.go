package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// 交叉编译好的 Linux 节点程序，随客户端一起分发，部署时通过 SSH 上传。
//
//go:embed assets/openremote-node-linux-amd64
var nodeBinaryAMD64 []byte

//go:embed assets/openremote-node-linux-arm64
var nodeBinaryARM64 []byte

// DeployRequest 是"添加服务器并部署转发节点"的入参。
type DeployRequest struct {
	Host     string `json:"host"`
	SSHPort  int    `json:"sshPort"`
	User     string `json:"user"`
	Password string `json:"password"`
	NodePort int    `json:"nodePort"`
}

// testSSHConnection 只验证 SSH 能否登录并执行命令，不修改服务器。
func testSSHConnection(host string, port int, user string, password string) error {
	if host == "" {
		return fmt.Errorf("缺少主机地址")
	}
	if port <= 0 {
		port = 22
	}
	if user == "" {
		user = "root"
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)), &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	out, err := runCommand(client, "uname -s; echo __OPENREMOTE_OK__")
	if err != nil {
		return fmt.Errorf("SSH 命令执行失败: %w（输出: %s）", err, strings.TrimSpace(out))
	}

	text := strings.TrimSpace(out)
	system := strings.TrimSpace(strings.Split(text, "__OPENREMOTE_OK__")[0])
	if !strings.Contains(text, "__OPENREMOTE_OK__") {
		return fmt.Errorf("SSH 已登录，但服务器没有执行 shell 命令（禁止远程执行或非标准 shell）")
	}
	if system == "" {
		// uname 无输出时用 /etc/os-release 兜底判断。
		fallback, _ := runCommand(client, `/bin/uname -s 2>/dev/null; head -n 5 /etc/os-release 2>/dev/null`)
		if strings.Contains(strings.ToLower(fallback), "linux") {
			return nil
		}
		system = strings.TrimSpace(fallback)
	}
	if !strings.Contains(strings.ToLower(system), "linux") {
		if system == "unknown" || system == "" {
			return fmt.Errorf("无法识别服务器操作系统（uname 无输出）")
		}
		return fmt.Errorf("目前只支持 Linux 服务器（当前系统: %s）", system)
	}
	return nil
}

// deployNodeViaSSH 通过 SSH 检查并安装 openremote-node，然后启动它，返回节点的 WebSocket 地址。
func deployNodeViaSSH(req DeployRequest) (string, error) {
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return "", fmt.Errorf("缺少服务器地址")
	}
	sshPort := req.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	nodePort := req.NodePort
	if nodePort == 0 {
		nodePort = 9000
	}
	user := req.User
	if user == "" {
		user = "root"
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(req.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(sshPort)), config)
	if err != nil {
		return "", fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	machine, _ := runCommand(client, "uname -m")
	binary := nodeBinaryAMD64
	arch := strings.ToLower(machine)
	if strings.Contains(arch, "aarch64") || strings.Contains(arch, "arm64") {
		binary = nodeBinaryARM64
	}

	nodePath := `"$HOME/.openremote/openremote-node"`
	logPath := `"$HOME/.openremote/node.log"`

	// 先检查服务器上是否已经安装了同版本的节点，已安装则直接复用，避免重复上传。
	remoteVersion, _ := runCommand(client, nodePath+" --version 2>/dev/null")
	installed := strings.Contains(remoteVersion, nodeVersion)

	if installed {
		running, _ := runCommand(client, "pgrep -x openremote-node >/dev/null 2>&1 && echo running || echo stopped")
		if strings.Contains(running, "running") {
			if err := waitForPort(host, nodePort, 3*time.Second); err == nil {
				return fmt.Sprintf("ws://%s:%d/ws", host, nodePort), nil
			}
		}
		startCmd := fmt.Sprintf(
			"(pkill -x openremote-node >/dev/null 2>&1 || true) && "+
				"nohup %s --addr :%d > %s 2>&1 < /dev/null &",
			nodePath, nodePort, logPath,
		)
		if _, err := runCommand(client, startCmd); err != nil {
			return "", fmt.Errorf("启动节点失败: %w", err)
		}
	} else {
		if err := uploadBinary(client, binary); err != nil {
			return "", fmt.Errorf("上传节点程序失败: %w", err)
		}
		startCmd := fmt.Sprintf(
			"chmod +x %s.new && mv %s.new %s && (pkill -x openremote-node >/dev/null 2>&1 || true) && "+
				"nohup %s --addr :%d > %s 2>&1 < /dev/null &",
			nodePath, nodePath, nodePath, nodePath, nodePort, logPath,
		)
		if _, err := runCommand(client, startCmd); err != nil {
			return "", fmt.Errorf("启动节点失败: %w", err)
		}
	}

	// 尽量放通节点端口（ufw / firewalld 已启用时），避免服务在跑但外部连不上。
	allowFirewallPort(client, nodePort)

	if err := waitForPort(host, nodePort, 4*time.Second); err != nil {
		logs, _ := runCommand(client, fmt.Sprintf("tail -n 20 %s 2>/dev/null", logPath))
		return "", fmt.Errorf("节点未在 %d 端口就绪: %w\n%s", nodePort, err, strings.TrimSpace(logs))
	}

	return fmt.Sprintf("ws://%s:%d/ws", host, nodePort), nil
}

// uploadBinary 上传节点程序。
// 二进制约 6MB，单条 SSH 通道串行传输在高延迟服务器上很慢，因此先 gzip 压缩，
// 再拆成 4 段用并发通道上传，最后在服务器上合并解压。
func uploadBinary(client *ssh.Client, binary []byte) error {
	if _, err := runCommand(client, `mkdir -p "$HOME/.openremote"`); err != nil {
		return err
	}

	payload, compressed := gzipPayload(binary)

	uploadErr := uploadChunks(client, payload, 4)
	if uploadErr == nil {
		var merge string
		if compressed {
			merge = `cat "$HOME/.openremote/.upload-part-"* | gzip -dc > "$HOME/.openremote/openremote-node.new"`
		} else {
			merge = `cat "$HOME/.openremote/.upload-part-"* > "$HOME/.openremote/openremote-node.new"`
		}
		merge += ` && rm -f "$HOME/.openremote/.upload-part-"*`
		if _, err := runCommand(client, merge); err == nil {
			return nil
		} else {
			uploadErr = err
		}
	}

	// 并发/压缩方案失败时回退到单通道直传，保证仍能完成部署。
	_, _ = runCommand(client, `rm -f "$HOME/.openremote/.upload-part-"*`)
	if uploadErr != nil {
		session, err := client.NewSession()
		if err != nil {
			return err
		}
		defer session.Close()
		session.Stdin = bytes.NewReader(binary)
		return session.Run(`cat > "$HOME/.openremote/openremote-node.new"`)
	}
	return nil
}

func gzipPayload(binary []byte) ([]byte, bool) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return binary, false
	}
	if _, err := writer.Write(binary); err != nil {
		return binary, false
	}
	if err := writer.Close(); err != nil {
		return binary, false
	}
	return buf.Bytes(), true
}

func uploadChunks(client *ssh.Client, payload []byte, count int) error {
	if len(payload) == 0 {
		return errors.New("空数据")
	}

	size := (len(payload) + count - 1) / count
	var wg sync.WaitGroup
	errCh := make(chan error, count)

	for index := 0; index < count; index++ {
		start := index * size
		if start >= len(payload) {
			break
		}
		end := start + size
		if end > len(payload) {
			end = len(payload)
		}

		wg.Add(1)
		go func(index int, data []byte) {
			defer wg.Done()

			session, err := client.NewSession()
			if err != nil {
				errCh <- err
				return
			}
			defer session.Close()

			session.Stdin = bytes.NewReader(data)
			if err := session.Run(fmt.Sprintf(`cat > "$HOME/.openremote/.upload-part-%d"`, index)); err != nil {
				errCh <- err
			}
		}(index, payload[start:end])
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func allowFirewallPort(client *ssh.Client, port int) {
	value := strconv.Itoa(port)
	commands := []string{
		fmt.Sprintf(`(command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active" && ufw allow %s/tcp >/dev/null 2>&1) || true`, value),
		fmt.Sprintf(`(command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --permanent --add-port=%s/tcp >/dev/null 2>&1 && firewall-cmd --reload >/dev/null 2>&1) || true`, value),
	}
	for _, command := range commands {
		_, _ = runCommand(client, command)
	}
}

func runCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var output bytes.Buffer
	session.Stdout = &output
	session.Stderr = &output
	err = session.Run(command)
	return output.String(), err
}

func waitForPort(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}
