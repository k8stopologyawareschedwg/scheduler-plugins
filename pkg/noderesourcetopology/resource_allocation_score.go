package noderesourcetopology

import (
	"context"
	"fmt"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	apiconfig "sigs.k8s.io/scheduler-plugins/pkg/apis/config"
)

type ScoreStrategy string

const (
	// ScorePluginName is the name of the plugin used in the plugin registry and configurations.
	ScorePluginName = "NodeResourceTopologyResourceAllocationScore"

	// LeastAllocatable strategy favors node with the least amount of available resource
	LeastAllocatable ScoreStrategy = "least-allocatable"

	// BalancedAllocation strategy favors nodes with balanced resource usage rate
	BalancedAllocation ScoreStrategy = "balanced-allocation"

	// MostAllocatable strategy favors node with the most amount of available resource
	MostAllocatable ScoreStrategy = "most-allocatable"

)

// resourceAllocationScorer contains information to calculate resource allocation score.
type resourceAllocationScorer struct {
	scoreStrategy       scoreStrategy
	resourceToWeightMap resourceToWeightMap
	NodeResTopoPlugin
}

var _ framework.ScorePlugin = &resourceAllocationScorer{}

func (r *resourceAllocationScorer) Name() string {
	return ScorePluginName
}

func (r *resourceAllocationScorer) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// calculates the fraction of requested to capacity per each numa-node.
	// return the numa-node with the minimal score as the node's total score
	return r.score(pod, nodeName)
}

func (r *resourceAllocationScorer) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// score will use `scoreStrategy` function to calculate the score.
func (r *resourceAllocationScorer) score(pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	klog.V(5).Infof("Call score for node %v", nodeName)
	nodeTopology := findNodeTopology(nodeName, &r.NodeResTopoPlugin)

	if nodeTopology == nil {
		return 0, nil
	}

	klog.V(5).Infof("nodeTopology: %v", nodeTopology)
	calculateResourceAllocatableRequest(pod, nodeTopology)
	resources, allocatablePerNUMA := calculateResourceAllocatableRequest(pod, nodeTopology)
	return scoreForEachNUMANode(resources, allocatablePerNUMA, r.scoreStrategy, r.resourceToWeightMap)
}

// scoreForEachNUMANode will iterate over all NUMA zones of the node and invoke the scoreStrategy func for every zone.
// it will return the minimal score of all the calculated NUMA's score, in order to avoid edge cases.
func scoreForEachNUMANode(requested v1.ResourceList, numaList NUMANodeList, score scoreStrategy, resourceToWeightMap resourceToWeightMap ) (int64, *framework.Status) {
	numaScores := make([]int64, len(numaList))
	for _, numa := range numaList {
		numaScores[numa.NUMAID] = score(requested, numa.Resources, resourceToWeightMap)
	}
	klog.V(5).Infof("numaScores: %v", numaScores)
	minScore := findMinScore(numaScores)
	klog.V(5).Infof("node score: %v", minScore)
	return minScore, nil
}

func calculateResourceAllocatableRequest(pod *v1.Pod, nodeTopology *topologyv1alpha1.NodeResourceTopology) (v1.ResourceList, NUMANodeList) {
	containers := append(pod.Spec.InitContainers, pod.Spec.Containers...)
	resources := make(v1.ResourceList)

	for _, container := range containers {
		for resource, quantity := range container.Resources.Requests {
			if quan, ok := resources[resource]; ok {
				quantity.Add(quan)
			}
			resources[resource] = quantity
		}
	}

	allocatable := createNUMANodeList(nodeTopology.Zones)
	return resources, allocatable
}

func findMinScore(arr []int64) int64 {
	min := arr[0]

	for _, score := range arr {
		// if NUMA's score is 0, i.e. not fit at all, it won't be take under consideration by Kubelet.
		if (min == 0) || (score != 0 && score < min) {
			min = score
		}
	}
	return min
}

func getScoreStrategy(strategy string) scoreStrategy {
	switch ScoreStrategy(strategy) {
	case LeastAllocatable:
		return leastAllocatableScoreStrategy
	case MostAllocatable:
		return mostAllocatableScoreStrategy
	case BalancedAllocation:
		return balancedAllocationScoreStrategy
	// TBD what strategy should be provided by default
	default:
		return leastAllocatableScoreStrategy

	}
}

// NewResourceAllocationScore initializes a new plugin and returns it.
func NewResourceAllocationScore(args runtime.Object, handle framework.FrameworkHandle) (framework.Plugin, error) {
	klog.V(5).Infof("creating new NewResourceAllocationScore plugin")
	raArgs, ok := args.(*apiconfig.NodeResourceTopologyResourceAllocationScoreArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type NodeResourceTopologyResourceAllocationScoreArgs, got %T", args)
	}

	lister, err := getNodeTopologyLister(&raArgs.MasterOverride, &raArgs.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	resToWeightMap := make(resourceToWeightMap)
	for _, resource := range raArgs.Resources {
		resToWeightMap[v1.ResourceName(resource.Name)] = resource.Weight
	}

	NewResourceAllocationScorer := &resourceAllocationScorer{
		scoreStrategy: getScoreStrategy(raArgs.ScoreSchedulingStrategy),
		NodeResTopoPlugin: NodeResTopoPlugin{
			Lister:     lister,
			Namespaces: raArgs.Namespaces,
		},
		resourceToWeightMap: resToWeightMap,
	}

	return NewResourceAllocationScorer, nil
}
