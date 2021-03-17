package noderesourcetopology

import (
	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

type commonPluginsData struct {
	lister     listerv1alpha1.NodeResourceTopologyLister
	namespaces []string
}

type nodeTopologyMap map[string]topologyv1alpha1.NodeResourceTopology

type NUMANode struct {
	NUMAID    int
	Resources v1.ResourceList
}

type NUMANodeList []NUMANode
