package noderesourcetopology

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

func leastAllocatableScoreStrategy(requested, allocatable v1.ResourceList, resourceToWeightMap resourceToWeightMap) int64 {
	var numaNodeScore int64 = 0
	var weightSum int64 = 0

	for resourceName := range requested {
		// We don't care what kind of resources are being requested, we just iterate all of them.
		// If NUMA zone doesn't have the requested resource, the score for that resource will be 0.
		resourceScore := leastAllocatableScore(requested[resourceName], allocatable[resourceName])
		weight := resourceToWeightMap.GetWeight(resourceName)
		numaNodeScore += resourceScore * weight
		weightSum += weight
	}

	return numaNodeScore / weightSum
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
