package cp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/unifabric-io/nvair-cli/pkg/config"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	sshpkg "github.com/unifabric-io/nvair-cli/pkg/ssh"
)

func TestParseCopyLocation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantFound bool
		wantNode  string
		wantPath  string
		wantErr   string
	}{
		{
			name:      "remote path",
			input:     "node-gpu-1:/tmp/test.txt",
			wantFound: true,
			wantNode:  "node-gpu-1",
			wantPath:  "/tmp/test.txt",
		},
		{
			name:      "local path containing colon",
			input:     "/tmp/local:file.txt",
			wantFound: false,
		},
		{
			name:    "missing node name",
			input:   ":/tmp/test.txt",
			wantErr: "invalid remote path",
		},
		{
			name:    "missing remote path",
			input:   "node-gpu-1:",
			wantErr: "invalid remote path",
		},
		{
			name:      "remote path with colon",
			input:     "node-gpu-1:/tmp/key:value",
			wantFound: true,
			wantNode:  "node-gpu-1",
			wantPath:  "/tmp/key:value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := parseCopyLocation(tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if !found {
				return
			}
			if got.NodeName != tc.wantNode || got.Path != tc.wantPath {
				t.Fatalf("got remote = %+v, want node=%q path=%q", got, tc.wantNode, tc.wantPath)
			}
		})
	}
}

func TestRegister_SimulationFlagOptional(t *testing.T) {
	cc := NewCommand()
	cmd := &cobra.Command{
		Use:           "cp <src> <dest>",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cc.Execute(args)
		},
	}
	cc.Register(cmd)
	if flag := cmd.Flags().Lookup("api-endpoint"); flag != nil {
		t.Fatalf("did not expect api-endpoint flag to be registered")
	}
	cmd.SetArgs([]string{"node-gpu-1:/tmp/a.txt", "/tmp/a.txt"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected command execution error")
	}
	if strings.Contains(err.Error(), "required flag(s) \"simulation\" not set") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_UploadUsesResolvedCredentials(t *testing.T) {
	server := newCopyTestServer(t, []map[string]interface{}{
		{
			"id":         "node-1",
			"name":       "node-gpu-1",
			"state":      "RUNNING",
			"metadata":   `{"mgmt_ip":"192.168.200.6"}`,
			"os":         "img-ubuntu",
			"simulation": "sim-1",
		},
	}, []map[string]interface{}{{"id": "img-ubuntu", "name": "generic/ubuntu2404"}})
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "upload.txt")
	if err := os.WriteFile(localFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	oldKeyPathFn := defaultKeyPathFn
	defaultKeyPathFn = func() (string, error) { return "/tmp/mock-key", nil }
	t.Cleanup(func() { defaultKeyPathFn = oldKeyPathFn })

	oldUploadFn := copyFileViaBastionFn
	oldDownloadFn := copyFileFromBastionFn
	t.Cleanup(func() {
		copyFileViaBastionFn = oldUploadFn
		copyFileFromBastionFn = oldDownloadFn
	})

	var gotCfg sshpkg.BastionCopyConfig
	var gotLocal, gotRemote string
	copyFileViaBastionFn = func(cfg sshpkg.BastionCopyConfig, localPath, remotePath string) error {
		gotCfg = cfg
		gotLocal = localPath
		gotRemote = remotePath
		return nil
	}
	copyFileFromBastionFn = func(cfg sshpkg.BastionCopyConfig, remotePath, localPath string) error {
		t.Fatalf("unexpected download invocation")
		return nil
	}

	cc := NewCommand()
	cc.APIEndpoint = server.URL
	cc.SimulationName = ""

	err := cc.Execute([]string{localFile, "node-gpu-1:/tmp/remote.txt"})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	if gotLocal != localFile || gotRemote != "/tmp/remote.txt" {
		t.Fatalf("copy args mismatch: local=%q remote=%q", gotLocal, gotRemote)
	}
	if gotCfg.BastionAddr != "worker01.air.nvidia.com:10022" {
		t.Fatalf("unexpected bastion addr: %s", gotCfg.BastionAddr)
	}
	if gotCfg.TargetAddr != "192.168.200.6:22" {
		t.Fatalf("unexpected target addr: %s", gotCfg.TargetAddr)
	}
	if gotCfg.TargetUser != constant.DefaultUbuntuUser || gotCfg.TargetPass != constant.DefaultUbuntuPassword {
		t.Fatalf("unexpected target credentials: user=%q pass=%q", gotCfg.TargetUser, gotCfg.TargetPass)
	}
	if gotCfg.DirectTarget {
		t.Fatalf("expected non-direct target")
	}
}

func TestExecute_DownloadFromBastionNodeUsesDirectTarget(t *testing.T) {
	server := newCopyTestServer(t, []map[string]interface{}{
		{
			"id":         "node-1",
			"name":       constant.OOBMgmtServerName,
			"state":      "RUNNING",
			"metadata":   `{"mgmt_ip":"192.168.200.2"}`,
			"os":         "img-ubuntu",
			"simulation": "sim-1",
		},
	}, []map[string]interface{}{{"id": "img-ubuntu", "name": "generic/ubuntu2404"}})
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	oldKeyPathFn := defaultKeyPathFn
	defaultKeyPathFn = func() (string, error) { return "/tmp/mock-key", nil }
	t.Cleanup(func() { defaultKeyPathFn = oldKeyPathFn })

	oldUploadFn := copyFileViaBastionFn
	oldDownloadFn := copyFileFromBastionFn
	t.Cleanup(func() {
		copyFileViaBastionFn = oldUploadFn
		copyFileFromBastionFn = oldDownloadFn
	})

	var gotCfg sshpkg.BastionCopyConfig
	var gotRemote, gotLocal string
	copyFileViaBastionFn = func(cfg sshpkg.BastionCopyConfig, localPath, remotePath string) error {
		t.Fatalf("unexpected upload invocation")
		return nil
	}
	copyFileFromBastionFn = func(cfg sshpkg.BastionCopyConfig, remotePath, localPath string) error {
		gotCfg = cfg
		gotRemote = remotePath
		gotLocal = localPath
		return nil
	}

	cc := NewCommand()
	cc.APIEndpoint = server.URL
	cc.SimulationName = ""

	dst := filepath.Join(t.TempDir(), "download.txt")
	err := cc.Execute([]string{constant.OOBMgmtServerName + ":/var/log/cloud-init.log", dst})
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	if gotRemote != "/var/log/cloud-init.log" || gotLocal != dst {
		t.Fatalf("copy args mismatch: remote=%q local=%q", gotRemote, gotLocal)
	}
	if !gotCfg.DirectTarget {
		t.Fatalf("expected direct target for oob-mgmt-server")
	}
	if gotCfg.TargetPass != "" {
		t.Fatalf("expected empty target password for direct target, got %q", gotCfg.TargetPass)
	}
}

func TestExecute_Validation(t *testing.T) {
	cc := NewCommand()
	cc.SimulationName = "simple"

	err := cc.Execute([]string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "either source or destination must be remote") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = cc.Execute([]string{"node-a:/tmp/a", "node-b:/tmp/b"})
	if err == nil || !strings.Contains(err.Error(), "cannot both be remote") {
		t.Fatalf("unexpected error: %v", err)
	}

	dir := t.TempDir()
	err = cc.Execute([]string{dir, "node-a:/tmp/a"})
	if err == nil || !strings.Contains(err.Error(), "copying directories is not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_RequiresSimulationWhenMultipleExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/simulations" && r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 2,
				"results": []map[string]interface{}{
					{"id": "sim-1", "title": "simple", "state": "RUNNING"},
					{"id": "sim-2", "title": "lab-b", "state": "RUNNING"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupConfig(t, server.URL, "bearer-token", time.Now().Add(1*time.Hour))

	tmpDir := t.TempDir()
	localFile := filepath.Join(tmpDir, "upload.txt")
	if err := os.WriteFile(localFile, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}

	cc := NewCommand()
	cc.APIEndpoint = server.URL
	cc.SimulationName = ""

	err := cc.Execute([]string{localFile, "node-gpu-1:/tmp/remote.txt"})
	if err == nil {
		t.Fatalf("expected simulation selection validation error")
	}
	if !strings.Contains(err.Error(), "--simulation <name> is required (2 simulations found)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newCopyTestServer(t *testing.T, nodes []map[string]interface{}, images []map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/simulations" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count": 1,
				"results": []map[string]interface{}{
					{"id": "sim-1", "title": "simple", "state": "RUNNING"},
				},
			})
		case r.URL.Path == "/v1/service" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"id":           "svc-1",
					"service_type": "ssh",
					"host":         "worker01.air.nvidia.com",
					"src_port":     10022,
				},
			})
		case r.URL.Path == "/v2/simulations/nodes/" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count":   len(nodes),
				"results": nodes,
			})
		case r.URL.Path == "/v2/images" && r.Method == "GET":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"count":   len(images),
				"results": images,
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func setupConfig(t *testing.T, endpoint, bearer string, expiresAt time.Time) {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := &config.Config{
		Username:             "user@example.com",
		APIToken:             "api-token",
		BearerToken:          bearer,
		BearerTokenExpiresAt: expiresAt,
		APIEndpoint:          endpoint,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path error: %v", err)
	}
	if _, err := os.Stat(filepath.Clean(path)); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
}
