package bastion

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// BastionExecConfig holds the configuration for executing commands via bastion host
type BastionExecConfig struct {
	BastionUser string
	BastionAddr string
	BastionKey  string

	TargetUser string
	TargetAddr string
	TargetPass string

	Command string
}

// ExecResult holds the result of command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

var scriptPreInstallDocker = `bash -c "
set -e

sudo modprobe br_netfilter

sudo apt install -y linux-modules-extra-$(uname -r)
sudo ip link set eth1 up
sudo rdma link add rxe1 type rxe netdev eth1

sudo apt-get update -y
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings

sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: \$(. /etc/os-release && echo \${UBUNTU_CODENAME:-\$VERSION_CODENAME})
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
EOF

sudo apt-get update -y
"
`

var scriptInstallDocker = `bash -c "
set -e
sudo swapoff -a && sudo sed -i '/swap/d' /etc/fstab
sudo apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin -y
containerd config default | sudo tee /etc/containerd/config.toml
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
sudo systemctl restart containerd
sudo systemctl restart docker
"
`

// ExecCommandViaBastion executes a command on target host via bastion host
// It establishes connection through bastion using public key authentication and uses password auth to target
func ExecCommandViaBastion(cfg BastionExecConfig) (*ExecResult, error) {
	bastionClient, err := dialBastion(
		cfg.BastionUser,
		cfg.BastionAddr,
		cfg.BastionKey,
	)
	if err != nil {
		return nil, err
	}
	defer bastionClient.Close()

	conn, err := bastionClient.Dial("tcp", cfg.TargetAddr)
	if err != nil {
		return nil, fmt.Errorf("dial target via bastion failed: %w", err)
	}

	targetClient, err := newTargetClient(
		conn,
		cfg.TargetAddr,
		cfg.TargetUser,
		cfg.TargetPass,
	)
	if err != nil {
		return nil, err
	}
	defer targetClient.Close()

	return execCommand(targetClient, cfg.Command)
}

// ExecCommandOnBastion executes a command directly on bastion host
func ExecCommandOnBastion(cfg BastionExecConfig) (*ExecResult, error) {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return nil, err
	}
	defer bastionClient.Close()
	return execCommand(bastionClient, cfg.Command)
}

// newTargetClient creates an SSH client connection to target host through bastion
func newTargetClient(
	conn net.Conn,
	addr, user, pass string,
) (*ssh.Client, error) {

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("new ssh client conn failed: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// dialBastion creates an SSH client connection to bastion host using public key authentication
func dialBastion(user, addr, keyPath string) (*ssh.Client, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return ssh.Dial("tcp", addr, cfg)
}

// execCommand executes a command on SSH client and returns the result
func execCommand(client *ssh.Client, cmd string) (*ExecResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(cmd)

	exitCode := 0
	if err != nil {
		// 命令执行失败，但 SSH 成功
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return nil, err
		}
	}

	return &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}, nil
}

// WaitPingViaBastion waits for target host to be reachable via bastion with ping
func WaitPingViaBastion(ctx context.Context, cfg BastionExecConfig, timeout time.Duration) error {
	host := cfg.TargetAddr
	if h, _, err := net.SplitHostPort(cfg.TargetAddr); err == nil {
		host = h
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pingCfg := cfg
		pingCfg.Command = fmt.Sprintf("ping -c1 -W2 %s", host)
		fmt.Println(pingCfg.Command, "...")
		res, err := ExecCommandOnBastion(pingCfg)
		if err == nil && res != nil && res.ExitCode == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// InstallDockerViaBastion installs Docker on target host via bastion host
// It runs pre-installation setup and then installs Docker
func InstallDockerViaBastion(cfg BastionExecConfig, pingTimeout time.Duration) (*ExecResult, *ExecResult, error) {
	if err := WaitPingViaBastion(context.Background(), cfg, pingTimeout); err != nil {
		return nil, nil, fmt.Errorf("ping target failed: %w", err)
	}

	preCfg := cfg
	preCfg.Command = scriptPreInstallDocker
	preRes, err := ExecCommandViaBastion(preCfg)
	if err != nil {
		return preRes, nil, err
	}
	if preRes.ExitCode != 0 {
		return preRes, nil, fmt.Errorf("pre-install docker failed: %s", preRes.Stderr)
	}

	installCfg := cfg
	installCfg.Command = scriptInstallDocker
	installRes, err := ExecCommandViaBastion(installCfg)
	if err != nil {
		return preRes, installRes, err
	}
	if installRes.ExitCode != 0 {
		return preRes, installRes, fmt.Errorf("install docker failed: %s", installRes.Stderr)
	}

	return preRes, installRes, nil
}

// Kubeadm installation and initialization scripts
var installKubeadmSh = `bash -c "
set -e
sudo systemctl stop unattended-upgrades

sudo apt-get install -y apt-transport-https ca-certificates curl
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.35/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.35/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl

sudo systemctl enable --now kubelet
"`

var initKubeadmSh = `bash -c "
set -e
sudo kubeadm init \
--pod-network-cidr=10.200.0.0/16 \
--service-cidr=10.201.0.0/16 \
--cri-socket=unix:///var/run/containerd/containerd.sock

mkdir -p \$HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf \$HOME/.kube/config
sudo chown \$(id -u):\$(id -g) \$HOME/.kube/config

kubectl taint nodes --all node-role.kubernetes.io/control-plane-
"`

// InstallKubeadmViaBastion installs kubeadm on target host via bastion
func InstallKubeadmViaBastion(cfg BastionExecConfig) (*ExecResult, error) {
	installCfg := cfg
	installCfg.Command = installKubeadmSh
	res, err := ExecCommandViaBastion(installCfg)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("install kubeadm failed: %s", res.Stderr)
	}
	return res, nil
}

// InitKubeadmViaBastion initializes kubeadm on the first controller node
func InitKubeadmViaBastion(cfg BastionExecConfig) (*ExecResult, error) {
	initCfg := cfg
	initCfg.Command = initKubeadmSh
	res, err := ExecCommandViaBastion(initCfg)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("kubeadm init failed: %s", res.Stderr)
	}
	return res, nil
}

// GetKubeadmJoinCommandViaBastion retrieves the kubeadm join command from the master node
func GetKubeadmJoinCommandViaBastion(cfg BastionExecConfig) (string, *ExecResult, error) {
	joinCfg := cfg
	joinCfg.Command = "sudo kubeadm token create --print-join-command --ttl 24h"
	res, err := ExecCommandViaBastion(joinCfg)
	if err != nil {
		return "", res, err
	}
	if res.ExitCode != 0 {
		return "", res, fmt.Errorf("get join command failed: %s", res.Stderr)
	}

	cmd := strings.TrimSpace(res.Stdout)
	return cmd, res, nil
}

// JoinKubeadmViaBastion joins a worker node to the kubeadm cluster
func JoinKubeadmViaBastion(cfg BastionExecConfig) (*ExecResult, error) {
	res, err := ExecCommandViaBastion(cfg)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("kubeadm join failed: %s", res.Stderr)
	}
	return res, nil
}

// ForwardServer represents a port forwarding configuration for nginx
type ForwardServer struct {
	Name       string
	LocalPort  int
	RemoteIP   string
	RemotePort int
}

var installNginxScript = `bash -c "
set -e
sudo systemctl stop apache2
sudo systemctl disable apache2
sudo apt update
sudo apt purge -y nginx nginx-core nginx-light nginx-common
sudo apt autoremove -y
sudo DEBIAN_FRONTEND=noninteractive apt install -y nginx-extras
sudo systemctl restart nginx
sudo systemctl enable nginx
"
`

// InstallNginx installs nginx on the bastion host
func InstallNginx(ctx context.Context, cfg BastionExecConfig) error {
	installCfg := cfg
	installCfg.Command = installNginxScript
	res, err := ExecCommandOnBastion(installCfg)
	if err != nil {
		return fmt.Errorf("install nginx failed: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("install nginx failed: %s", strings.TrimSpace(res.Stderr))
	}
	return nil
}

// UpdateNginxConfig updates nginx configuration with port forwarding rules
func UpdateNginxConfig(ctx context.Context, cfg BastionExecConfig, list []ForwardServer) (*ExecResult, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("no forward servers provided")
	}

	var stream strings.Builder
	stream.WriteString("stream {\n")
	for _, fs := range list {
		fmt.Fprintf(&stream, "    server { listen 0.0.0.0:%d; proxy_pass %s:%d; }\n", fs.LocalPort, fs.RemoteIP, fs.RemotePort)
	}
	stream.WriteString("}\n")

	fullConf := fmt.Sprintf(`
load_module /usr/lib/nginx/modules/ngx_stream_module.so;
worker_processes auto;
error_log /var/log/nginx/error.log;
pid /run/nginx.pid;

events { worker_connections 1024; }

%s`, stream.String())

	script := fmt.Sprintf(`bash -c '
set -e
sudo tee /etc/nginx/nginx.conf >/dev/null <<"EOF"
%sEOF
sudo nginx -t
sudo systemctl restart nginx
'`, fullConf)

	execCfg := cfg
	execCfg.Command = script
	return ExecCommandOnBastion(execCfg)
}

// GetKubeconfigViaBastion retrieves the kubeconfig from the master node
func GetKubeconfigViaBastion(cfg BastionExecConfig) (string, error) {
	cfg.Command = "cat ~/.kube/config"
	result, err := ExecCommandViaBastion(cfg)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}
