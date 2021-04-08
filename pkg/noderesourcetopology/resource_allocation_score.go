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

const (
	// TODO create dynamic customizable resource weight
	defaultResourceWeight int64 = 1
	// Name is the name of the plugin used in the plugin registry and configurations.
	ResourceAllocationName = "NodeResourceTopologyResourceAllocationScore"
)

// resourceToWeightMap contains resource name and weight.
type resourceToWeightMap map[v1.ResourceName]int64

type scoreStrategy func(v1.ResourceList, v1.ResourceList) int64

// resourceToValueMap contains resource name and score.
type resourceToValueMap map[v1.ResourceName]int64

// resourceAllocationScorer contains information to calculate resource allocation score.
type resourceAllocationScorer struct {
	scoreStrategy       scoreStrategy
	resourceToWeightMap resourceToWeightMap
	data                commonPluginsData
}

// defaultRequestedRatioResources is used to set default requestToWeight map for CPU and memory
var defaultRequestedRatioResources = resourceToWeightMap{v1.ResourceMemory: 1, v1.ResourceCPU: 1}

var _ framework.ScorePlugin = &resourceAllocationScorer{}

func (r *resourceAllocationScorer) Name() string {
	return ResourceAllocationName
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
	nodeTopology := findNodeTopology(nodeName, &r.data)

	if nodeTopology == nil {
		return 0, nil
	}

	klog.V(5).Infof("nodeTopology: %v", nodeTopology)
	calculateResourceAllocatableRequest(pod, nodeTopology)
	resources, allocatablePerNUMA := calculateResourceAllocatableRequest(pod, nodeTopology)
	return scoreHelper(resources, allocatablePerNUMA, r.scoreStrategy)
}

// scoreHelper will iterate over all NUMA zones of the node and invoke the scoreStrategy func for every zone.
// Eventually it will return the minimal score of all the calculated NUMA's score, in order to avoid edge cases.
func scoreHelper(requested v1.ResourceList, numaList NUMANodeList, score scoreStrategy) (int64, *framework.Status) {
	numaScores := make([]int64, len(numaList))
	for _, numa := range numaList {
		numaScores[numa.NUMAID] = score(requested, numa.Resources)
	}

	return findMinScore(numaScores), nil
}

func calculateResourceAllocatableRequest(pod *v1.Pod, nodeTopology *topologyv1alpha1.NodeResourceTopology) (v1.ResourceList, NUMANodeList) {
	containers := []v1.Container{}
	containers = append(pod.Spec.InitContainers, pod.Spec.Containers...)
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
	var min int64 = framework.MaxNodeScore
	for _, score := range arr {
		if score < min {
			min = score
		}
	}
	return min
}

func getScoreStrategy(strategy string) scoreStrategy {
	switch strategy {
	case "leastAllocatable":
		return leastAllocatableScoreStrategy()
	case "mostAllocatable":
		return mostAllocatableScoreStrategy()
	case "balancedAllocation":
		return balancedAllocationScoreStrategy()
	// TBD what strategy should be provided by default
	default:
		return leastAllocatableScoreStrategy()

	}
}

// NewResourceAllocationScore initializes a new plugin and returns it.
func NewResourceAllocationScore(args runtime.Object, handle framework.FrameworkHandle) (framework.Plugin, error) {
	klog.V(5).Infof("creating new NewResourceAllocationScore plugin")
	raArgs, ok := args.(*apiconfig.ResourceAllocationScoreArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type ResourceAllocationScoreArgs, got %T", args)
	}

	nodeTopologyInformer, err := getNodeTopologyInformer(&raArgs.MasterOverride, &raArgs.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	NewResourceAllocationScorer := &resourceAllocationScorer{
		scoreStrategy: getScoreStrategy(raArgs.ScoreSchedulingStrategy),
		data: commonPluginsData{
			lister:     (*nodeTopologyInformer).Lister(),
			namespaces: raArgs.Namespaces,
		},
	}

	return NewResourceAllocationScorer, nil
}
