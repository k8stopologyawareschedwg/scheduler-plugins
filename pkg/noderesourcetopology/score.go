package noderesourcetopology

import (
	"context"
	"fmt"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
	apiconfig "sigs.k8s.io/scheduler-plugins/pkg/apis/config"
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	ScorerName = "NumaNodeScorer"

	// TODO create dynamic customizable resource weight
	defaultResourceWeight int64 = 1

	// Maximal score that a node may have
	MaxNodeScore int64 = 100
)

type NumaNodeScorer struct {
	data commonPluginsData
}

var _ framework.ScorePlugin = &NumaNodeScorer{}

func (nns *NumaNodeScorer) Name() string {
	return ScorerName
}

func (nns *NumaNodeScorer) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	klog.V(5).Infof("Call score for node %v", nodeName)
	nodeTopology := findNodeTopology(nodeName, &nns.data)

	if nodeTopology == nil {
		return 0, nil
	}

	klog.V(5).Infof("nodeTopology: %v", nodeTopology)

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

	// calculates the fraction of requested to capacity per each numa-node.
	// return the numa-node with the minimal score as the node's total score
	return score(pod, nodeTopology.Zones, resources)
}

func score(pod *v1.Pod, zones topologyv1alpha1.ZoneList, requested v1.ResourceList) (int64, *framework.Status) {
	numaList := createNUMANodeList(zones)
	numaScores := make([]int64, len(numaList))
	for _, numa := range numaList {
		numaScores[numa.NUMAID] = leastAllocatableScorer(requested, numa.Resources)
	}

	return findMinScore(numaScores), nil
}

func leastAllocatableScorer(requested, allocatable v1.ResourceList) int64 {
	var numaNodeScore int64 = 0
	var weightSum int64 = 0
	var weight int64 = defaultResourceWeight

	for resourceName := range requested {
		// We don't care what kind of resources are being requested, we just iterate all of them.
		// If numa-node doesn't have the requested resource, the score for that resource will be 0.
		resourceScore := leastAllocatableScore(requested[resourceName], allocatable[resourceName])
		numaNodeScore += resourceScore * weight
		weightSum++
	}

	return (numaNodeScore / int64(weightSum))
}

// The used capacity is calculated on a scale of 0-MaxNodeScore (MaxNodeScore is
// constant with value set to 100).
// 0 being the lowest priority and 100 being the highest.
// The less allocatable resources the node has, the higher the score is.
func leastAllocatableScore(requested, numaCapacity resource.Quantity) int64 {
	if numaCapacity.CmpInt64(0) == 0 {
		return 0
	}
	if requested.Cmp(numaCapacity) > 0 {
		return 0
	}

	return (requested.Value() * framework.MaxNodeScore) / numaCapacity.Value()
}

func findMinScore(arr []int64) int64 {
	var min int64 = MaxNodeScore
	for _, score := range arr {
		if score < min {
			min = score
		}
	}
	return min
}

func (nns *NumaNodeScorer) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// NewNumaNodeScorer initializes a new plugin and returns it.
func NewNumaNodeScorer(args runtime.Object, handle framework.FrameworkHandle) (framework.Plugin, error) {
	klog.V(5).Infof("creating new NumaNodeScorer plugin")
	nnsArgs, ok := args.(*apiconfig.NumaNodeScorerArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type NodeResourceTopologyMatchArgs, got %T", args)
	}

	nodeTopologyInformer, err := initNodeTopologyInformer(nnsArgs.MasterOverride, nnsArgs.KubeConfigPath)
	if err != nil {
		return nil, err
	}

	numaNodeScorer := &NumaNodeScorer{
		data: commonPluginsData{
			lister:     nodeTopologyInformer.Lister(),
			namespaces: nnsArgs.Namespaces,
		},
	}

	return numaNodeScorer, nil
}
