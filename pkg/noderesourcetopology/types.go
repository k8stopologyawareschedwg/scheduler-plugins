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

const defaultWeight = int64(1)

// GetWeight return the weight of the resource and defaultWeight if weight not specified
func(rw *resourceToWeightMap) GetWeight(r v1.ResourceName) int64{
	w, ok := (*rw)[r]
	if !ok {
		return defaultWeight
	}
	return w
}

type scoreStrategy func(v1.ResourceList, v1.ResourceList, resourceToWeightMap) int64
