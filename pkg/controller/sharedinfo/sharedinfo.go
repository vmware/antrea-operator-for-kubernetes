package sharedinfo

import (
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type SharedInfo struct {
	sync.Mutex
	AntreaAgentDaemonSetSpec       *unstructured.Unstructured
	AntreaControllerDeploymentSpec *unstructured.Unstructured
}

func New() *SharedInfo {
	return &SharedInfo{}
}
