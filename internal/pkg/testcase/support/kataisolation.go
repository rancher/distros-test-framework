package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	core "k8s.io/api/core/v1"
)

// Isolation implements KATA-14, not penetration testing or a security certification.
func (s *KataRun) Isolation(ctx context.Context) (runErr error) {
	pod := s.pods["kata-local"]
	if kernelErr := s.guestKernel(ctx, pod); kernelErr != nil {
		return kernelErr
	}

	marker, markerErr := s.createHostMarker(ctx)
	if markerErr != nil {
		return markerErr
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cleanupErr := s.hostMarker(cleanupCtx, "cleanup")
		runErr = errors.Join(runErr, cleanupErr)
	}()
	_, probeErr := s.exec(ctx, pod, "sh", "-ec", guestIsolation, "probe", marker.Path,
		strconv.Itoa(marker.PID), marker.Netns, s.ns, marker.Device)
	if probeErr != nil {
		return fmt.Errorf("host resource isolation in pod %s: %w", pod.Name, probeErr)
	}
	logKata("pod %s cannot see the host marker file, host PID %d, network namespace %s, /dev/%s or /dev/kvm",
		pod.Name, marker.PID, marker.Netns, marker.Device)

	if mountErr := s.guestMount(ctx); mountErr != nil {
		return mountErr
	}
	verifyErr := s.hostMarker(ctx, "verify")
	if verifyErr != nil {
		return fmt.Errorf("host marker disappeared or guest operation escaped: %w", verifyErr)
	}

	return s.evidence("isolation-host-markers", marker)
}

const guestIsolation = `test -d /proc && test -d /sys && test -d /dev
test ! -e "$1/token" && test ! -e "/proc/1/root$1/token"
test ! -e "/run/netns/$3" && test ! -e /dev/kvm
test ! -e "/dev/$5" && test ! -e "/sys/class/block/$5"
if test -r "/proc/$2/cmdline"; then
  if tr '\000' ' ' < "/proc/$2/cmdline" | grep -F "$4"; then exit 1; fi
fi`

func (s *KataRun) mountPod() core.Pod {
	pod := s.workload("guest-mount", s.eligible, kataClass, s.opts.Image)
	pod.Namespace = s.ns + "-isolation"
	pod.Spec.SecurityContext.RunAsNonRoot = kataPtr(false)
	pod.Spec.SecurityContext.RunAsUser = kataPtr(int64(0))
	pod.Spec.SecurityContext.RunAsGroup = kataPtr(int64(0))
	c := &pod.Spec.Containers[0]
	c.SecurityContext.Capabilities.Add = []core.Capability{"SYS_ADMIN"}
	c.SecurityContext.SeccompProfile = &core.SeccompProfile{Type: core.SeccompProfileTypeUnconfined}
	c.ReadinessProbe = &core.Probe{
		ProbeHandler: core.ProbeHandler{
			Exec: &core.ExecAction{
				Command: []string{"sh", "-ec", `test "$(cat "$GUEST_PATH/token")" = "$TOKEN"`},
			},
		},
		PeriodSeconds: 2,
	}
	c.Env = []core.EnvVar{
		{
			Name:  "GUEST_PATH",
			Value: "/tmp/" + s.ns + "-guest",
		},
		{
			Name:  "TOKEN",
			Value: s.ns,
		},
	}
	c.Args = []string{`test ! -e "$GUEST_PATH"; mkdir "$GUEST_PATH"
mount -t tmpfs -o size=1m,nodev,nosuid,noexec tmpfs "$GUEST_PATH"
printf '%s' "$TOKEN" > "$GUEST_PATH/token"; exec sleep 3600`}

	return pod
}

func (s *KataRun) guestMount(ctx context.Context) error {
	pod := s.mountPod()
	if err := s.namespace(ctx, pod.Namespace, "privileged"); err != nil {
		return err
	}
	if err := s.create(ctx, "guest-mount", pod); err != nil {
		return err
	}

	ready, err := s.ready(ctx, pod.Namespace, pod.Name)
	if err != nil {
		return err
	}
	record := &kataPod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		IP:        s.eligibleIP,
		Runtime:   "kata",
		UID:       string(ready.UID),
	}
	record.Identity, err = s.identity(ctx, record)
	if err != nil {
		return err
	}

	_, err = s.exec(ctx, record, "sh", "-ec",
		`grep " $1 " /proc/mounts; test "$(cat "$1/token")" = "$2"`, "probe", "/tmp/"+s.ns+"-guest", s.ns)
	if err != nil {
		return fmt.Errorf("guest privileged operation must succeed inside the VM: %w", err)
	}
	if verifyErr := s.hostMarker(ctx, "verify"); verifyErr != nil {
		return fmt.Errorf("host changed while the guest mount was active: %w", verifyErr)
	}
	logKata("pod %s: privileged tmpfs mount inside the Kata guest worked; host markers unchanged", record.Name)

	return s.deletePod(ctx, record)
}

var (
	kataMarkerName = regexp.MustCompile(`^dtf-kata-[a-z0-9]+$`)
	kataDeviceName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

type kataHostMarker struct {
	Path, Netns, Device string
	PID                 int
}

func (s *KataRun) createHostMarker(ctx context.Context) (*kataHostMarker, error) {
	if !kataMarkerName.MatchString(s.ns) {
		return nil, errors.New("invalid isolation marker name")
	}

	output, err := s.node(ctx, s.eligibleIP, "lsblk", "-J", "-d", "-o", "NAME,TYPE")
	if err != nil {
		return nil, fmt.Errorf("list host block devices for isolation probe: %w", err)
	}

	var devices struct{ BlockDevices []struct{ Name, Type string } }
	if decodeErr := json.Unmarshal([]byte(output), &devices); decodeErr != nil {
		return nil, fmt.Errorf("decode host block devices for isolation probe: %w", decodeErr)
	}

	device := ""
	for _, item := range devices.BlockDevices {
		if item.Type == "disk" && kataDeviceName.MatchString(item.Name) {
			device = item.Name
			break
		}
	}
	if device == "" {
		return nil, errors.New("no host block device for the isolation probe")
	}

	output, err = s.node(ctx, s.eligibleIP, "sudo", "-n", "bash", "-ec", kataMarkerCreate, "probe", s.ns, device)
	if err != nil {
		return nil, fmt.Errorf("create host isolation markers: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || pid < 1 {
		return nil, errors.Join(errors.New("invalid synthetic host PID"), s.hostMarker(ctx, "cleanup"))
	}

	return &kataHostMarker{
		PID:    pid,
		Path:   "/var/tmp/" + s.ns,
		Netns:  s.ns,
		Device: device,
	}, nil
}

func (s *KataRun) hostMarker(ctx context.Context, mode string) error {
	if !kataMarkerName.MatchString(s.ns) || (mode != "verify" && mode != "cleanup") {
		return errors.New("invalid host marker operation")
	}
	_, err := s.node(ctx, s.eligibleIP, "sudo", "-n", "bash", "-ec", kataMarkerCheck, "probe", s.ns, mode)
	if err != nil {
		return fmt.Errorf("%s host isolation markers: %w", mode, err)
	}

	return nil
}

const kataMarkerCreate = `root=/var/tmp/"$1"
test ! -e /tmp/"$1"-guest && test ! -e /run/netns/"$1"
mkdir -m 755 "$root"
pid=; netns=false
cleanup() {
  test -z "$pid" || kill "$pid" 2>/dev/null || true
  if "$netns"; then ip netns delete "$1"; fi
  rm -f -- "$root/token" "$root/pid" "$root/device"
  rmdir "$root"
}
trap 'cleanup "$1"' EXIT
printf '%s' "$1" > "$root/token"
chmod 644 "$root/token"
printf '%s' "$2" > "$root/device"
ip netns add "$1"; netns=true
nohup bash -c 'exec -a "$1" sleep 3600' marker "$1" </dev/null >/dev/null 2>&1 &
pid=$!
printf '%s' "$pid" > "$root/pid"
for attempt in 1 2 3 4 5; do
  tr '\000' '\n' < /proc/"$pid"/cmdline | grep -Fx -- "$1" >/dev/null && break
  sleep 0.1
done
tr '\000' '\n' < /proc/"$pid"/cmdline | grep -Fx -- "$1" >/dev/null
test -b /dev/"$2" && test -e /run/netns/"$1"
printf '%s\n' "$pid"
trap - EXIT`

const kataMarkerCheck = `root=/var/tmp/"$1"
test ! -L "$root" && test -d "$root"
test "$(cat "$root/token")" = "$1"
pid=$(cat "$root/pid")
case "$pid" in ''|*[!0-9]*) exit 1;; esac
device=$(cat "$root/device")
case "$device" in ''|*[!a-zA-Z0-9_-]*) exit 1;; esac
if test "$2" = verify; then
  tr '\000' '\n' < /proc/"$pid"/cmdline | grep -Fx -- "$1" >/dev/null
  test -e /run/netns/"$1" && test -b /dev/"$device"
  test ! -e /tmp/"$1"-guest
  if awk -v path="/tmp/$1-guest" '$2 == path {found=1} END {exit !found}' /proc/mounts; then exit 1; fi
else
  if test -r /proc/"$pid"/cmdline && tr '\000' '\n' < /proc/"$pid"/cmdline | grep -Fx -- "$1" >/dev/null; then
    kill "$pid"
  fi
  ip netns delete "$1"
  rm -- "$root/token" "$root/pid" "$root/device"
  rmdir "$root"
fi`
