package rancher

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// FleetNamespace is where Rancher keeps provisioning clusters, snapshots and control planes.
const FleetNamespace = "fleet-default"

type condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

type phaseStatus struct {
	Generation int `json:"generation"`
}

// ControlPlane is the subset of rke.cattle.io/v1 RKEControlPlane status the tests wait on.
type ControlPlane struct {
	Status struct {
		Conditions               []condition  `json:"conditions"`
		ETCDSnapshotCreate       *phaseStatus `json:"etcdSnapshotCreate"`
		ETCDSnapshotCreatePhase  string       `json:"etcdSnapshotCreatePhase"`
		ETCDSnapshotRestore      *phaseStatus `json:"etcdSnapshotRestore"`
		ETCDSnapshotRestorePhase string       `json:"etcdSnapshotRestorePhase"`
	} `json:"status"`
}

// ParseControlPlane decodes `kubectl get rkecontrolplane -o json` output.
func ParseControlPlane(raw string) (*ControlPlane, error) {
	var cp ControlPlane
	if err := json.Unmarshal([]byte(raw), &cp); err != nil {
		return nil, fmt.Errorf("decode rkecontrolplane: %w", err)
	}

	return &cp, nil
}

// Ready reports whether the Ready condition is True.
func (cp *ControlPlane) Ready() bool {
	for _, c := range cp.Status.Conditions {
		if c.Type == "Ready" {
			return c.Status == "True"
		}
	}

	return false
}

// SnapshotCreated reports whether snapshot request generation has finished on a ready control plane.
func (cp *ControlPlane) SnapshotCreated(generation int) bool {
	s := cp.Status
	return s.ETCDSnapshotCreate != nil && s.ETCDSnapshotCreate.Generation == generation &&
		s.ETCDSnapshotCreatePhase == "Finished" && cp.Ready()
}

// SnapshotRestored reports whether restore request generation has finished on a ready control plane.
func (cp *ControlPlane) SnapshotRestored(generation int) bool {
	s := cp.Status
	return s.ETCDSnapshotRestore != nil && s.ETCDSnapshotRestore.Generation == generation &&
		s.ETCDSnapshotRestorePhase == "Finished" && cp.Ready()
}

type snapshotList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		SnapshotFile struct {
			Name      string `json:"name"`
			Size      int64  `json:"size"`
			Status    string `json:"status"`
			CreatedAt string `json:"createdAt"`
		} `json:"snapshotFile"`
	} `json:"items"`
}

// SnapshotNames returns every etcdsnapshot object name in `kubectl get etcdsnapshots -o json`.
func SnapshotNames(raw string) (map[string]bool, error) {
	var l snapshotList
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return nil, fmt.Errorf("decode etcdsnapshots: %w", err)
	}
	names := make(map[string]bool, len(l.Items))
	for _, i := range l.Items {
		names[i.Metadata.Name] = true
	}

	return names, nil
}

// NewSnapshot returns the object and file name of the newest usable snapshot absent from before.
func NewSnapshot(raw string, before map[string]bool) (name, fileName string, err error) {
	var l snapshotList
	if err = json.Unmarshal([]byte(raw), &l); err != nil {
		return "", "", fmt.Errorf("decode etcdsnapshots: %w", err)
	}

	sort.SliceStable(l.Items, func(i, j int) bool {
		return l.Items[i].SnapshotFile.CreatedAt > l.Items[j].SnapshotFile.CreatedAt
	})
	for _, i := range l.Items {
		f := i.SnapshotFile
		if before[i.Metadata.Name] || f.Size == 0 || strings.EqualFold(f.Status, "failed") {
			continue
		}

		return i.Metadata.Name, f.Name, nil
	}

	return "", "", errors.New("no new successful etcd snapshot yet")
}

// RegistrationToken holds the registration commands Rancher generates. All fields are secrets.
type RegistrationToken struct {
	Token               string `json:"token"`
	TokenSecretName     string `json:"tokenSecretName"`
	InsecureNodeCommand string `json:"insecureNodeCommand"`
	InsecureCommand     string `json:"insecureCommand"`
}

// tokenPlaceholder is what Rancher 2.14+ puts in the commands; the token itself is in TokenSecretName.
const tokenPlaceholder = "{token}"

// NeedsSecret reports whether the commands are templates that need the token from TokenSecretName.
func (t *RegistrationToken) NeedsSecret() bool {
	return strings.Contains(t.InsecureNodeCommand+t.InsecureCommand, tokenPlaceholder)
}

// Fill substitutes the token read from TokenSecretName into the command templates.
func (t *RegistrationToken) Fill(token string) {
	t.Token = token
	t.InsecureNodeCommand = strings.ReplaceAll(t.InsecureNodeCommand, tokenPlaceholder, token)
	t.InsecureCommand = strings.ReplaceAll(t.InsecureCommand, tokenPlaceholder, token)
}

// Secrets returns every value that must stay out of logs and screenshots: the token and both commands.
func (t *RegistrationToken) Secrets() []string {
	return []string{t.Token, t.InsecureNodeCommand, t.InsecureCommand}
}

// ParseRegistrationToken returns the first token that has a registration command, from
// `kubectl get clusterregistrationtokens -o json`.
func ParseRegistrationToken(raw string) (*RegistrationToken, error) {
	var l struct {
		Items []struct {
			Status RegistrationToken `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return nil, fmt.Errorf("decode clusterregistrationtokens: %w", err)
	}
	for _, i := range l.Items {
		t := i.Status
		if t.InsecureNodeCommand == "" && t.InsecureCommand == "" {
			continue
		}
		if t.NeedsSecret() && t.TokenSecretName == "" {
			return nil, errors.New("registration commands are templates but name no token secret")
		}

		return &t, nil
	}

	return nil, errors.New("cluster registration token has no commands yet")
}

// ImportCommand rewrites Rancher's import command to use the node's own kubectl.
func ImportCommand(insecureCommand, kubectl string) (string, error) {
	const pipe = "| kubectl apply"
	if !strings.Contains(insecureCommand, pipe) {
		return "", errors.New("import command does not pipe into kubectl apply; Rancher format changed")
	}

	return strings.Replace(insecureCommand, pipe, "| "+kubectl+" apply", 1), nil
}

// ClusterManifest renders a provisioning.cattle.io/v1 Cluster. An empty version makes it an imported cluster.
func ClusterManifest(name, kubernetesVersion string) (string, error) {
	spec := map[string]any{}
	if kubernetesVersion != "" {
		spec["kubernetesVersion"] = kubernetesVersion
		spec["rkeConfig"] = map[string]any{}
	}
	b, err := json.Marshal(map[string]any{
		"apiVersion": "provisioning.cattle.io/v1",
		"kind":       "Cluster",
		"metadata":   map[string]any{"name": name, "namespace": FleetNamespace},
		"spec":       spec,
	})

	return string(b), err
}
