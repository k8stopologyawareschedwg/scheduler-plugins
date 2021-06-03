package noderesourcetopology

import (
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

type commonPluginsData struct {
	pLister    *listerv1alpha1.NodeResourceTopologyLister
	namespaces []string
}

type NUMANode struct {
	NUMAID    int
	Resources v1.ResourceList
}

type podRequests struct {
	pod        *v1.Pod
	name       string
	wantStatus *framework.Status
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
