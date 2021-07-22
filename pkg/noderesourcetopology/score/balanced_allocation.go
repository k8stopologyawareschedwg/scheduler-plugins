package score

import (
	"gonum.org/v1/gonum/stat"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

func balancedAllocationScoreStrategy(requested, allocatable v1.ResourceList, resourceToWeightMap resourceToWeightMap) int64 {
	resourceFractions := make([]float64, 0)

	// We don't care what kind of resources are being requested, we just iterate all of them.
	// If NUMA zone doesn't have the requested resource, the score for that resource will be 0.
	for resourceName := range requested {
		resourceFraction := fractionOfCapacity(requested[resourceName], allocatable[resourceName])
		// if requested > capacity the corresponding NUMA zone should never be preferred
		if resourceFraction > 1 {
			return 0
		}
		resourceFractions = append(resourceFractions, resourceFraction)
	}

	variance := stat.Variance(resourceFractions, nil)

	// Since the variance is between positive fractions, it will be positive fraction. 1-variance lets the
	// score to be higher for node which has least variance and multiplying it with `MaxNodeScore` provides the scaling
	// factor needed.
	return int64((1 - variance) * float64(framework.MaxNodeScore))
}

func fractionOfCapacity(requested, capacity resource.Quantity) float64 {
	if capacity.Value() == 0 {
		return 1
	}
	return float64(requested.Value()) / float64(capacity.Value())
}
