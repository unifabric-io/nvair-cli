# Service Port Forwarding Sync

**Status**: Technical Reference  
**Generated**: January 12, 2026

This document describes the `nvair forward sync` command that synchronizes Kubernetes NodePort services to NVIDIA Air service forwarding rules.

## Overview

When running Kubernetes clusters in NVIDIA Air simulations, NodePort services are not directly accessible from outside the simulation. The `nvair forward sync` command automatically:
1. Ensures nginx is installed on the bastion host (oob-mgmt-server)
2. Queries **all** K8s NodePort services from **all namespaces** on the control plane node via SSH
3. Configures nginx on the bastion to proxy/forward traffic to NodePort services
4. Creates Air service forwarding rules with `k8s-` prefix for external access

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │  Kubernetes Cluster                 │
                    │  ┌────────────────────────────────┐ │
                    │  │ Control Plane (Master) Node    │ │
                    │  │  - kubectl get svc (remote)    │ │
                    │  └────────────────────────────────┘ │
                    │  ┌────────────────────────────────┐ │
                    │  │ NodePort Services              │ │
                    │  │  - my-web-app:30080            │ │
                    │  │  - my-api:30443                │ │
                    │  └────────────────────────────────┘ │
                    └─────────────────────────────────────┘
                                    ↑
                                    │ nginx proxy
                    ┌───────────────┴─────────────────────┐
                    │  Bastion Host (oob-mgmt-server)     │
                    │  - nginx installed & configured     │
                    │  - stream blocks for each NodePort  │
                    └─────────────────────────────────────┘
                                    ↑
                                    │ Air Service Forwarding
                    ┌───────────────┴─────────────────────┐
                    │  External Access (Public Internet)  │
                    │  worker01.air.nvidia.com:30080      │
                    └─────────────────────────────────────┘
```

## Command Usage

```bash
# Sync all NodePort services across all namespaces
# Specify the K8s control plane node name
nvair forward sync -s <simulation-name> --control-plane <node-name>
```

## Workflow

### Step 0: Ensure Nginx is Installed on Bastion

```go
func ensureNginxOnBastion(bastionHost BastionHost) error {
    // 1. Connect to bastion via SSH
    bastionClient := connectToBastion(bastionHost)
    session := bastionClient.NewSession()
    
    // 2. Check if nginx is installed
    output, err := session.Run("which nginx")
    if err == nil && strings.Contains(output, "nginx") {
        fmt.Println("✓ Nginx already installed on bastion")
        return nil
    }
    
    // 3. Install nginx
    fmt.Println("Installing nginx on bastion...")
    installCmds := []string{
        "sudo apt-get update",
        "sudo apt-get install -y nginx",
        "sudo systemctl enable nginx",
        "sudo systemctl start nginx",
    }
    
    for _, cmd := range installCmds {
        _, err := session.Run(cmd)
        if err != nil {
            return fmt.Errorf("failed to install nginx: %w", err)
        }
    }
    
    fmt.Println("✓ Nginx installed successfully")
    return nil
}
```

### Step 1: Query K8s NodePort Services via Remote kubectl

```go
func getNodePortServices(controlPlaneNode Node, bastionHost BastionHost) ([]K8sNodePortService, error) {
    // 1. Connect to bastion
    bastionClient := connectToBastion(bastionHost)
    
    // 2. SSH from bastion to control plane node
    cpSession := bastionClient.NewSession(controlPlaneNode.ManagementIP)
    
    // 3. Execute kubectl get svc across all namespaces
    kubectlCmd := "kubectl get svc --all-namespaces -o json"
    
    output, err := cpSession.Run(kubectlCmd)
    if err != nil {
        return nil, fmt.Errorf("failed to get services: %w", err)
    }
    
    // 4. Parse JSON output
    var serviceList struct {
        Items []struct {
            Metadata struct {
                Name      string            `json:"name"`
                Namespace string            `json:"namespace"`
                Labels    map[string]string `json:"labels"`
            } `json:"metadata"`
            Spec struct {
                Type     string `json:"type"`
                Selector map[string]string `json:"selector"`
                Ports    []struct {
                    Name       string `json:"name"`
                    Protocol   string `json:"protocol"`
                    Port       int    `json:"port"`
                    TargetPort int    `json:"targetPort"`
                    NodePort   int    `json:"nodePort"`
                } `json:"ports"`
            } `json:"spec"`
        } `json:"items"`
    }
    
    json.Unmarshal([]byte(output), &serviceList)
    
    // 5. Filter NodePort services
    var nodePortSvcs []K8sNodePortService
    for _, svc := range serviceList.Items {
        if svc.Spec.Type == "NodePort" {
            for _, port := range svc.Spec.Ports {
                if port.NodePort > 0 {
                    nodePortSvcs = append(nodePortSvcs, K8sNodePortService{
                        Namespace:  svc.Metadata.Namespace,
                        Name:       svc.Metadata.Name,
                        NodePort:   port.NodePort,
                        TargetPort: port.TargetPort,
                        Protocol:   port.Protocol,
                        Selector:   svc.Spec.Selector,
                        Labels:     svc.Metadata.Labels,
                    })
                }
            }
        }
    }
    
    return nodePortSvcs, nil
}
```

### Step 2: Configure Nginx on Bastion

```go
func configureNginxForServices(bastionHost BastionHost, k8sServices []K8sNodePortService, controlPlaneIP string) error {
    // 1. Generate nginx stream configuration
    nginxConfig := generateNginxStreamConfig(k8sServices, controlPlaneIP)
    
    // 2. Connect to bastion
    bastionClient := connectToBastion(bastionHost)
    session := bastionClient.NewSession()
    
    // 3. Write nginx configuration file
    configPath := "/etc/nginx/streams.d/k8s-nodeports.conf"
    writeCmd := fmt.Sprintf("sudo tee %s > /dev/null <<'EOF'\n%s\nEOF", configPath, nginxConfig)
    _, err := session.Run(writeCmd)
    if err != nil {
        return fmt.Errorf("failed to write nginx config: %w", err)
    }
    
    // 4. Test nginx configuration
    _, err = session.Run("sudo nginx -t")
    if err != nil {
        return fmt.Errorf("nginx config test failed: %w", err)
    }
    
    // 5. Reload nginx
    _, err = session.Run("sudo systemctl reload nginx")
    if err != nil {
        return fmt.Errorf("failed to reload nginx: %w", err)
    }
    
    return nil
}

func generateNginxStreamConfig(services []K8sNodePortService, upstreamIP string) string {
    var config strings.Builder
    
    config.WriteString("# Auto-generated by nvair forward sync\n")
    config.WriteString("# K8s NodePort service forwarding\n\n")
    
    for _, svc := range services {
        streamName := fmt.Sprintf("%s_%s_%d", svc.Namespace, svc.Name, svc.NodePort)
        
        // Determine protocol
        protocol := "tcp"
        if strings.ToLower(svc.Protocol) == "udp" {
            protocol = "udp"
        }
        
        config.WriteString(fmt.Sprintf("stream {\n"))
        config.WriteString(fmt.Sprintf("    upstream %s {\n", streamName))
        config.WriteString(fmt.Sprintf("        server %s:%d;\n", upstreamIP, svc.NodePort))
        config.WriteString(fmt.Sprintf("    }\n\n"))
        config.WriteString(fmt.Sprintf("    server {\n"))
        config.WriteString(fmt.Sprintf("        listen %d %s;\n", svc.NodePort, protocol))
        config.WriteString(fmt.Sprintf("        proxy_pass %s;\n", streamName))
        config.WriteString(fmt.Sprintf("    }\n"))
        config.WriteString(fmt.Sprintf("}\n\n"))
    }
    
    return config.String()
}
```

### Step 3: Create Air Service Forwarding Rules

```go
func createAirServicesForBastion(bastionNode Node, k8sServices []K8sNodePortService, simulationID string) (*ForwardSyncResult, error) {
    result := &ForwardSyncResult{}
    
    // Get bastion's management interface
    bastionInterface, err := getBastionInterface(simulationID, bastionNode.ID)
    if err != nil {
        return nil, fmt.Errorf("failed to get bastion interface: %w", err)
    }
    
    // Create Air service for each NodePort
    for _, k8sSvc := range k8sServices {
        // Determine service type from protocol
        serviceType := "tcp"
        if strings.ToLower(k8sSvc.Protocol) == "udp" {
            serviceType = "udp"
        }
        
        // Create Air service with k8s- prefix
        payload := map[string]interface{}{
            "name":         fmt.Sprintf("k8s-%s-%s-%d", k8sSvc.Namespace, k8sSvc.Name, k8sSvc.NodePort),
            "simulation":   simulationID,
            "interface":    bastionInterface.ID,
            "dest_port":    k8sSvc.NodePort,
            "service_type": serviceType,
        }
        
        resp, err := apiClient.Post("/v1/service", payload)
        if err != nil {
            result.Failed = append(result.Failed, ForwardSyncError{
                ServiceName: fmt.Sprintf("%s/%s:%d", k8sSvc.Namespace, k8sSvc.Name, k8sSvc.NodePort),
                Error:       err,
                Message:     fmt.Sprintf("Failed to create Air service: %v", err),
            })
            continue
        }
        
        var airSvc AirService
        json.Unmarshal(resp.Body, &airSvc)
        result.Created = append(result.Created, airSvc)
    }
    
    return result, nil
}
```

### Step 4: Complete Workflow

```go
func ForwardSync(simulationID, controlPlaneName string) error {
    // 1. Get bastion and control plane nodes
    bastionNode, err := findBastionNode(simulationID)
    if err != nil {
        return fmt.Errorf("bastion not found: %w", err)
    }
    
    controlPlaneNode, err := findNodeByName(simulationID, controlPlaneName)
    if err != nil {
        return fmt.Errorf("control plane node '%s' not found: %w", controlPlaneName, err)
    }
    
    // 2. Ensure nginx is installed on bastion
    fmt.Println("Checking bastion host...")
    if err := ensureNginxOnBastion(bastionNode); err != nil {
        return fmt.Errorf("nginx setup failed: %w", err)
    }
    
    // 3. Query all K8s NodePort services from all namespaces
    fmt.Printf("Querying K8s NodePort services from %s (all namespaces)...\n", controlPlaneName)
    k8sServices, err := getNodePortServices(controlPlaneNode, bastionNode)
    if err != nil {
        return fmt.Errorf("failed to query K8s services: %w", err)
    }
    fmt.Printf("Found %d NodePort services across all namespaces\n", len(k8sServices))
    
    if len(k8sServices) == 0 {
        fmt.Println("No NodePort services found. Nothing to sync.")
        return nil
    }
    
    // 4. Configure nginx on bastion
    fmt.Println("Configuring nginx on bastion...")
    if err := configureNginxForServices(bastionNode, k8sServices, controlPlaneNode.ManagementIP); err != nil {
        return fmt.Errorf("nginx configuration failed: %w", err)
    }
    
    // 5. Create Air service forwarding rules
    fmt.Println("Creating Air service forwarding rules...")
    result, err := createAirServicesForBastion(bastionNode, k8sServices, simulationID)
    if err != nil {
        return fmt.Errorf("failed to create Air services: %w", err)
    }
    
    // 6. Display results
    fmt.Printf("\nSummary:\n")
    fmt.Printf("  ✓ %d services synced\n", len(result.Created))
    if len(result.Failed) > 0 {
        fmt.Printf("  ✗ %d failed\n", len(result.Failed))
        for _, f := range result.Failed {
            fmt.Printf("    - %s: %s\n", f.ServiceName, f.Message)
        }
    }
    
    fmt.Println("\nAccess your services:")
    for _, svc := range result.Created {
        fmt.Printf("  - %s:    %s\n", svc.Name, svc.Link)
    }
    fmt.Println("\nNote: Traffic flows through bastion nginx proxy to NodePort services")
    
    return nil
}
```

---

## Output Example

```bash
$ nvair forward sync -s demo --control-plane k8s-master

Syncing Kubernetes NodePort services to Air...

Checking bastion host (oob-mgmt-server)...
✓ Bastion host found: 192.168.100.1
Checking nginx installation...
✓ Nginx already installed

Querying K8s NodePort services from k8s-master...
Executing: kubectl get svc --all-namespaces -o json
Found 3 NodePort services:
  - default/my-web-app:30080 (TCP) → 8080
  - default/my-api:30443 (TCP) → 443
  - monitoring/prometheus:30900 (TCP) → 9090

Configuring nginx on bastion...
Writing /etc/nginx/streams.d/k8s-nodeports.conf
Testing nginx configuration... ✓
Reloading nginx... ✓

Creating Air service forwarding rules...
[1/3] k8s-default-my-web-app-30080... ✓
      External URL: http://worker01.air.nvidia.com:17234
      
[2/3] k8s-default-my-api-30443... ✓
      External URL: http://worker01.air.nvidia.com:17235
      
[3/3] k8s-monitoring-prometheus-30900... ✓
      External URL: http://worker01.air.nvidia.com:17236

Summary:
  ✓ 3 services synced
  ✗ 0 failed

All services synced successfully!

Access your services (with k8s- prefix):
  - k8s-default-my-web-app-30080:       http://worker01.air.nvidia.com:17234
  - k8s-default-my-api-30443:           http://worker01.air.nvidia.com:17235
  - k8s-monitoring-prometheus-30900:    http://worker01.air.nvidia.com:17236

Note: Traffic flows through bastion nginx proxy to NodePort services
```

## Error Handling

### Common Errors

**Control Plane Node Not Found**:
```
Error: Control plane node 'k8s-master' not found
  Solution: Check node name with: nvair get node -s demo
```

**Kubectl Not Found on Control Plane**:
```
Error: kubectl command not found on k8s-master
  Cause: Kubernetes not installed or kubectl not in PATH
  Solution: Ensure kubeadm is installed on the control plane node
```

**Nginx Installation Failed**:
```
Error: Failed to install nginx on bastion
  Cause: apt-get failed
  Solution: Check bastion internet connectivity and disk space
```
  Solution: Ensure kubectl is configured and cluster is accessible
```

**No NodePort Services Found**:
```
Warning: No NodePort services found in cluster
  This is normal if you haven't deployed any services yet
  Sync completed with no changes
```

**API Rate Limiting**:
```
Error: Failed to create Air service for default/my-app:30080
  Cause: API rate limit exceeded (429 Too Many Requests)
  Solution: Wait 60 seconds and retry
```

**Insufficient Permissions**:
```
Error: Failed to list services in namespace 'production'
  Cause: Forbidden (403)
  Solution: Ensure kubeconfig has 'list services' permission
```

## Configuration

### Kubeconfig Location

By default, the CLI looks for kubeconfig in:
1. `--kubeconfig` flag
2. `KUBECONFIG` environment variable
3. `$HOME/.kube/config`

### Service Naming Convention

Air service names are generated as:
```
{namespace}-{service-name}-{nodeport}
```

Example: `default-my-web-app-30080`

This ensures uniqueness and makes it easy to identify the source service.

### Idempotency

The sync operation is idempotent:
- Existing Air services with matching names are not recreated
- Services are only created/deleted when there are actual changes
- Safe to run multiple times

## Testing

### Unit Tests

```go
func TestSyncServices(t *testing.T) {
    k8sServices := []K8sNodePortService{
        {Namespace: "default", Name: "web", NodePort: 30080, Protocol: "TCP"},
    }
    
    airServices := []AirService{}
    
    result, err := syncServices(k8sServices, airServices, "sim-123")
    
    assert.NoError(t, err)
    assert.Len(t, result.Created, 1)
    assert.Len(t, result.Deleted, 0)
    assert.Equal(t, "default-web-30080", result.Created[0].Name)
}
```

### Integration Tests

1. Create K8s NodePort service
2. Run sync command
3. Verify Air service is created with correct port mapping
4. Delete K8s service
5. Run sync again
6. Verify Air service is deleted

---

**References**:
- Kubernetes Service Types: https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport
- NVIDIA Air API: https://air.nvidia.com/api/
- client-go library: https://github.com/kubernetes/client-go
