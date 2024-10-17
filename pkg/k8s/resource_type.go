package k8s

//go:generate go-enum -f=$GOFILE --ptr --marshal --flag --nocase --mustparse --names

// ENUM(
// Node,
// Pod,
// Service,
// Deployment,
// StatefulSet,
// DaemonSet,
// ReplicaSet,
// ReplicationController,
// PodTemplate,
// ConfigMap,
// Secret,
// ServiceAccount,
// PersistentVolume,
// PersistentVolumeClaim,
// StorageClass,
// VolumeAttachment,
// CSIDriver,
// CSINode,
// Ingress,
// IngressClass,
// NetworkPolicy,
// Endpoints,
// EndpointSlice,
// LimitRange,
// ResourceQuota,
// Event,
// Lease,
// Role,
// RoleBinding,
// ClusterRole,
// ClusterRoleBinding,
// CustomResourceDefinition,
// HorizontalPodAutoscaler,
// CronJob,
// Job,
// PodDisruptionBudget,
// PriorityClass
// ).
type ResourceType string
