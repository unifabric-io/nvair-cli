package create

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/api"
	"github.com/unifabric-io/nvair-cli/pkg/bastion"
	"github.com/unifabric-io/nvair-cli/pkg/constant"
	"github.com/unifabric-io/nvair-cli/pkg/logging"
	"github.com/unifabric-io/nvair-cli/pkg/netplan"
	"github.com/unifabric-io/nvair-cli/pkg/node"
	"github.com/unifabric-io/nvair-cli/pkg/ssh"
	"github.com/unifabric-io/nvair-cli/pkg/topology"
)

func validateGenericUbuntuNodeNetplans(directory string, topo *topology.RawTopology) error {
	for nodeKey, rawNode := range topo.Content.Nodes {
		node, ok := rawNode.(map[string]interface{})
		if !ok {
			continue
		}

		osName, _ := node["os"].(string)
		if !strings.Contains(strings.ToLower(osName), "generic") {
			continue
		}

		nodeName := nodeKey
		if name, ok := node["name"].(string); ok && name != "" {
			nodeName = name
		}

		configPath := filepath.Join(directory, nodeName+".yaml")
		if _, err := netplan.LoadFile(configPath); err != nil {
			return fmt.Errorf("invalid netplan config for generic node %s: %w", nodeName, err)
		}

		logging.Verbose("Validated netplan config for generic node %s: %s", nodeName, configPath)
	}

	return nil
}

func uploadNetplanConfigs(directory string, nodeNetplanTargets []api.Node, bastionAddr, keyPath string) error {
	logging.Info("Uploading netplan configs on nodes via bastion...")

	if len(nodeNetplanTargets) == 0 {
		logging.Info("No compute nodes found for netplan upload, skipping")
		return nil
	}

	errCh := make(chan error, len(nodeNetplanTargets))
	var wg sync.WaitGroup

	for _, n := range nodeNetplanTargets {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()

			meta, err := node.ParseNodeMetadata(n.Metadata)
			if err != nil {
				logging.Verbose("Failed to parse metadata for node %s: %v", n.Name, err)
				errCh <- fmt.Errorf("parse metadata failed for node %s: %w", n.Name, err)
				return
			}

			fileName := n.Name
			if !strings.HasSuffix(fileName, ".yaml") {
				fileName += ".yaml"
			}
			configPath := filepath.Join(directory, fileName)

			if _, err := os.Stat(configPath); err != nil {
				if os.IsNotExist(err) {
					logging.Info("Netplan config not found for node %s at %s, skipping", n.Name, configPath)
					return
				}
				logging.Verbose("Stat netplan config failed for node %s: %v", n.Name, err)
				errCh <- fmt.Errorf("stat netplan config failed for node %s: %w", n.Name, err)
				return
			}

			tmpDst := "/tmp/dataplan"
			logging.Info("Copying netplan for node %s (%s)...", n.Name, meta.MgmtIP)
			if err := ssh.CopyFileViaBastion(ssh.BastionCopyConfig{
				BastionAddr: bastionAddr,
				BastionUser: constant.DefaultUbuntuUser,
				BastionKey:  keyPath,
				TargetAddr:  meta.MgmtIP + ":22",
				TargetUser:  constant.DefaultUbuntuUser,
				TargetPass:  constant.DefaultUbuntuPassword,
				Timeout:     60 * time.Second,
			}, configPath, tmpDst); err != nil {
				logging.Verbose("Failed to upload netplan for node %s: %v", n.Name, err)
				errCh <- fmt.Errorf("upload netplan for node %s failed: %w", n.Name, err)
				return
			}

			res, err := bastion.ExecCommandViaBastion(bastion.BastionExecConfig{
				BastionUser: constant.DefaultUbuntuUser,
				BastionAddr: bastionAddr,
				BastionKey:  keyPath,
				TargetUser:  constant.DefaultUbuntuUser,
				TargetAddr:  meta.MgmtIP + ":22",
				TargetPass:  constant.DefaultUbuntuPassword,
				Command:     fmt.Sprintf("sudo mv %s /etc/netplan/%s && sudo netplan apply", tmpDst, fileName),
			})
			if err != nil {
				logging.Verbose("Failed to apply netplan on node %s: %v", n.Name, err)
				errCh <- fmt.Errorf("apply netplan on node %s failed: %w", n.Name, err)
				return
			}
			if res != nil && res.ExitCode != 0 {
				logging.Verbose("Netplan apply stderr for node %s: %s", n.Name, res.Stderr)
				errCh <- fmt.Errorf("apply netplan on node %s failed with exit %d", n.Name, res.ExitCode)
				return
			}

			logging.Info("✓ Netplan uploaded and applied on node %s.", n.Name)
		}()
	}

	wg.Wait()
	close(errCh)
	if err := joinErrors(errCh); err != nil {
		return err
	}

	logging.Info("✓ Netplan configs uploaded and applied on nodes.")
	return nil
}
