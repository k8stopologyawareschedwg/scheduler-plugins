package noderesourcetopology

import (
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
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

// resourceToWeightMap contains resource name and weight.
type resourceToWeightMap map[v1.ResourceName]int64

type scoreStrategy func(v1.ResourceList, v1.ResourceList, resourceToWeightMap) int64
