package noderesourcetopology

import (
	v1 "k8s.io/api/core/v1"

	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
)

type NodeResTopoPlugin struct {
	Lister     *listerv1alpha1.NodeResourceTopologyLister
	Namespaces []string
}

type NUMANode struct {
	NUMAID    int
	Resources v1.ResourceList
}

type NUMANodeList []NUMANode
