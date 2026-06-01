# NVIDIA GPU Operator Documentation

## Qase Test Suite

- **Test Suite**: Internal project in Qase.io

## RKE2 Integration

- **Documentation**: https://docs.rke2.io/advanced#deploy-nvidia-operator

## NVIDIA Platform Support

- **Reference**:
To find the latest drivers refer to:
https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/platform-support.html#supported-operating-systems-and-kubernetes-platforms

## Instance Type Requirements

**IMPORTANT**: NVIDIA GPU tests require GPU-enabled EC2 instances.

### Supported GPU Instance Types (AWS):
- `g4dn.xlarge` - NVIDIA T4 GPU (recommended for testing)
- `g5.xlarge` - NVIDIA A10G GPU  
- `p3.2xlarge` - NVIDIA V100 GPU
- `p4d.24xlarge` - NVIDIA A100 GPU

## Driver Version Configuration

The NVIDIA driver version can be specified via the `NVIDIA_VERSION` environment variable.

### Behavior by OS:

**SLES (SUSE Linux Enterprise Server):**
- **Optional**: Version parameter is informational only (logs for reference)
- **Behavior**: Always installs latest available from SUSE repos
- **Reason**: SLES package versions include kernel-specific builds, making version pinning complex
- Driver installed from SUSE cloud repos (automatic registration)
- Compute-utils installed from NVIDIA CUDA repos (latest to match driver)
- **Note**: If version is specified, it's logged but latest is installed

**Ubuntu / RHEL:**
- **Required**: Must provide version for driver download from NVIDIA website
- **Example**: `NVIDIA_VERSION=580.95.05`
- Driver downloaded and installed from https://us.download.nvidia.com/tesla/
- Exact version specified will be downloaded and installed

### Usage:

Add to your `.env` file:
```bash
# For Ubuntu/RHEL - specific version required
NVIDIA_VERSION=580.95.05
```

Or pass directly to test:
```bash
go test ./entrypoint/nvidia/... -nvidiaVersion "580.95.05"
```