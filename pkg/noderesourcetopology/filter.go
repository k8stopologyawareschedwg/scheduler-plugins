/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package noderesourcetopology

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	v1qos "k8s.io/kubernetes/pkg/apis/core/v1/helper/qos"
	bm "k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	apiconfig "sigs.k8s.io/scheduler-plugins/pkg/apis/config"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
)

const (
	// FilterPluginName is the name of the plugin used in the plugin registry and configurations.
	FilterPluginName = "NodeResourceTopologyMatch"
)

var _ framework.FilterPlugin = &TopologyMatch{}

type PolicyHandler func(pod *v1.Pod, zoneMap topologyv1alpha1.ZoneList) *framework.Status

type PolicyHandlerMap map[topologyv1alpha1.TopologyManagerPolicy]PolicyHandler

// TopologyMatch plugin which run simplified version of TopologyManager's admit handler
type TopologyMatch struct {
	policyHandlers PolicyHandlerMap
	NodeResTopoPlugin
}

// Name FilterPluginName returns name of the plugin. It is used in logs, etc.
func (tm *TopologyMatch) Name() string {
	return FilterPluginName
}

func SingleNUMAContainerLevelHandler(pod *v1.Pod, zones topologyv1alpha1.ZoneList) *framework.Status {
	klog.V(5).Infof("Single NUMA node handler")

	// prepare NUMANodes list from zoneMap
	nodes := createNUMANodeList(zones)
	qos := v1qos.GetPodQOS(pod)

	// We count here in the way TopologyManager is doing it, IOW we put InitContainers
	// and normal containers in the one scope
	for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
		if resMatchNUMANodes(nodes, container.Resources.Requests, qos) {
			// definitely we can't align container, so we can't align a pod
			return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Cannot align container: %s", container.Name))
		}
	}
	return nil
}

// resMatchNUMANodes checks for sufficient resource, this function
// requires NUMANodeList with properly populated NUMANode, NUMAID should be in range 0-63
func resMatchNUMANodes(nodes NUMANodeList, resources v1.ResourceList, qos v1.PodQOSClass) bool {
	bitmask := bm.NewEmptyBitMask()
	// set all bits, each bit is a NUMA node, if resources couldn't be aligned
	// on the NUMA node, bit should be unset
	bitmask.Fill()

	zeroQuantity := resource.MustParse("0")
	for resource, quantity := range resources {
		// for each requested resource, calculate which NUMA slots are good fits, and then AND with the aggregated bitmask, IOW unset appropriate bit if we can't align resources, or set it
		// obvious, bits which are not in the NUMA id's range would be unset
		resourceBitmask := bm.NewEmptyBitMask()
		for _, numaNode := range nodes {
			numaQuantity, ok := numaNode.Resources[resource]
			// if can't find requested resource on the node - skip (don't set it as available NUMA node)
			// if unfound resource has 0 quantity probably this numa node can be considered
			if !ok && quantity.Cmp(zeroQuantity) != 0 {
				continue
			}
			// Check for the following:
			// 1. set numa node as possible node if resource is memory or Hugepages
			// 2. set numa node as possible node if resource is cpu and it's not guaranteed QoS, since cpu will flow
			// 3. set numa node as possible node if zero quantity for non existing resource was requested
			// 4. otherwise check amount of resources
			if resource == v1.ResourceMemory ||
				strings.HasPrefix(string(resource), v1.ResourceHugePagesPrefix) ||
				resource == v1.ResourceCPU && qos != v1.PodQOSGuaranteed ||
				quantity.Cmp(zeroQuantity) == 0 ||
				numaQuantity.Cmp(quantity) >= 0 {
				// possible to align resources on NUMA node
				resourceBitmask.Add(numaNode.NUMAID)
			}
		}
		bitmask.And(resourceBitmask)
		if bitmask.IsEmpty() {
			return true
		}
	}
	return bitmask.IsEmpty()
}

func SingleNUMAPodLevelHandler(pod *v1.Pod, zones topologyv1alpha1.ZoneList) *framework.Status {
	klog.V(5).Infof("Pod Level Resource handler")
	resources := make(v1.ResourceList)

	// We count here in the way TopologyManager is doing it, IOW we put InitContainers
	// and normal containers in the one scope
	for _, container := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
		for resource, quantity := range container.Resources.Requests {
			if q, ok := resources[resource]; ok {
				quantity.Add(q)
			}
			resources[resource] = quantity
		}
	}

	if resMatchNUMANodes(createNUMANodeList(zones), resources, v1qos.GetPodQOS(pod)) {
		// definitely we can't align container, so we can't align a pod
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Cannot align pod: %s", pod.Name))
	}
	return nil
}

// Filter Now only single-numa-node supported
func (tm *TopologyMatch) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if nodeInfo.Node() == nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("Node is nil %s", nodeInfo.Node().Name))
	}
	if v1qos.GetPodQOS(pod) == v1.PodQOSBestEffort {
		return nil
	}

	nodeName := nodeInfo.Node().Name
	nodeTopology := findNodeTopology(nodeName, &tm.NodeResTopoPlugin)

	if nodeTopology == nil {
		return nil
	}

	klog.V(5).Infof("nodeTopology: %v", nodeTopology)
	for _, policyName := range nodeTopology.TopologyPolicies {
		if handler, ok := tm.policyHandlers[topologyv1alpha1.TopologyManagerPolicy(policyName)]; ok {
			if status := handler(pod, nodeTopology.Zones); status != nil {
				return status
			}
		} else {
			klog.V(5).Infof("Handler for policy %s not found", policyName)
		}
	}
	return nil
}

// New initializes a new plugin and returns it.
func New(args runtime.Object, handle framework.FrameworkHandle) (framework.Plugin, error) {
	klog.V(5).Infof("creating new TopologyMatch plugin")
	tcfg, ok := args.(*apiconfig.NodeResourceTopologyMatchArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type NodeResourceTopologyMatchArgs, got %T", args)
	}

	lister, err := getNodeTopologyLister(&tcfg.MasterOverride, &tcfg.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	topologyMatch := &TopologyMatch{
		policyHandlers: PolicyHandlerMap{
			topologyv1alpha1.SingleNUMANodePodLevel:       SingleNUMAPodLevelHandler,
			topologyv1alpha1.SingleNUMANodeContainerLevel: SingleNUMAContainerLevelHandler,
		},
		NodeResTopoPlugin: NodeResTopoPlugin{
			Lister:     lister,
			Namespaces: tcfg.Namespaces,
		},
	}

	return topologyMatch, nil
}
