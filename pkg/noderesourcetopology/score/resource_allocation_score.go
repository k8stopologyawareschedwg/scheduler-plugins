package score

import (
	"context"
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	apiconfig "sigs.k8s.io/scheduler-plugins/pkg/apis/config"
	nrt "sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology"
	"sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology/pluginhelpers"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
)

type StrategyName string

type scoreStrategy func(v1.ResourceList, v1.ResourceList, resourceToWeightMap) int64

// resourceToWeightMap contains resource name and weight.
type resourceToWeightMap map[v1.ResourceName]int64

// weight return the weight of the resource and defaultWeight if weight not specified
func (rw *resourceToWeightMap) weight(r v1.ResourceName) int64 {
	w, ok := (*rw)[r]
	if !ok {
		return defaultWeight
	}
	return w
}

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "NodeResourceTopologyResourceAllocationScore"

	// LeastAllocatable strategy favors node with the least amount of available resource
	LeastAllocatable StrategyName = "least-allocatable"

	// BalancedAllocation strategy favors nodes with balanced resource usage rate
	BalancedAllocation StrategyName = "balanced-allocation"

	// MostAllocatable strategy favors node with the most amount of available resource
	MostAllocatable StrategyName = "most-allocatable"

	defaultWeight = int64(1)
)

// resourceAllocationScorer contains information to calculate resource allocation score.
type resourceAllocationScorer struct {
	scoreStrategy       scoreStrategy
	resourceToWeightMap resourceToWeightMap
	nrt.NodeResTopoPlugin
}

var _ framework.ScorePlugin = &resourceAllocationScorer{}

func (r *resourceAllocationScorer) Name() string {
	return Name
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
	nodeTopology := pluginhelpers.FindNodeTopology(nodeName, &r.NodeResTopoPlugin)

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
func scoreForEachNUMANode(requested v1.ResourceList, numaList nrt.NUMANodeList, score scoreStrategy, resourceToWeightMap resourceToWeightMap) (int64, *framework.Status) {
	numaScores := make([]int64, len(numaList))
	for _, numa := range numaList {
		numaScores[numa.NUMAID] = score(requested, numa.Resources, resourceToWeightMap)
	}
	klog.V(5).Infof("numaScores: %v", numaScores)
	minScore := findMinScore(numaScores)
	klog.V(5).Infof("node score: %v", minScore)
	return minScore, nil
}

func calculateResourceAllocatableRequest(pod *v1.Pod, nodeTopology *topologyv1alpha1.NodeResourceTopology) (v1.ResourceList, nrt.NUMANodeList) {
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

	allocatable := pluginhelpers.CreateNUMANodeList(nodeTopology.Zones)
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
	switch StrategyName(strategy) {
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

// New initializes a new plugin and returns it.
func New(args runtime.Object, handle framework.FrameworkHandle) (framework.Plugin, error) {
	klog.V(5).Infof("creating new New plugin")
	raArgs, ok := args.(*apiconfig.NodeResourceTopologyResourceAllocationScoreArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type NodeResourceTopologyResourceAllocationScoreArgs, got %T", args)
	}

	lister, err := pluginhelpers.GetNodeTopologyLister(&raArgs.MasterOverride, &raArgs.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	resToWeightMap := make(resourceToWeightMap)
	for _, resource := range raArgs.Resources {
		resToWeightMap[v1.ResourceName(resource.Name)] = resource.Weight
	}

	NewResourceAllocationScorer := &resourceAllocationScorer{
		scoreStrategy: getScoreStrategy(raArgs.ScoreSchedulingStrategy),
		NodeResTopoPlugin: nrt.NodeResTopoPlugin{
			Lister:     lister,
			Namespaces: raArgs.Namespaces,
		},
		resourceToWeightMap: resToWeightMap,
	}

	return NewResourceAllocationScorer, nil
}
