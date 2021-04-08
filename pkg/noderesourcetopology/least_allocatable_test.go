package noderesourcetopology

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// return the name of the node with the highest score
func findMaxScoreNode(nodeToScoreMap map[string]int64) string {
	max := int64(0)
	electedNode := ""

	for nodeName, score := range nodeToScoreMap {
		if max < score {
			max = score
			electedNode = nodeName
		}
	}
	return electedNode
}

// recalcResources substruct the amount of requested resources from
// the node and updates the NodeResourceTopology object.
// Since we're running on a testing environment there is no actual assignment, thus we need to
// emulate that by updating the correct amount of available resources after each request
func recalcResources(pod *v1.Pod, node *v1.Node) {
	for _, container := range pod.Spec.Containers {
		for resourceName, reqQuan := range container.Resources.Requests {
			if allocQuan, ok := node.Status.Allocatable[resourceName]; ok {
				allocQuan.Sub(reqQuan)
				node.Status.Allocatable[resourceName] = allocQuan
			}
		}
	}
}

// updateNrt playing the role of Kubelet here.
// It will decide to which NUMA node, the pod will assigned.
// What the function does:
// 1. It will look for a NUMA node that can fit.
// (IMPORTANT NOTE: we don't really care which NUMA it will be, since wether it's NUMA 0 or NUMA 1,
// from the scoring plugin functionality perspective it won't make any difference)
// 2. It will fetch the fitted NUMA node object from the NodeResourceTopology
// 3. Update the NUMA node's available resources
func updateNrt(pod *v1.Pod, ntr *topologyv1alpha1.NodeResourceTopology) error {
	var fittedNUMAId int = -1
	numaList := createNUMANodeList(ntr.Zones)

	for _, numa := range numaList {
		if isNumaFit(pod, &numa) {
			fittedNUMAId = numa.NUMAID
			break
		}
	}
	if fittedNUMAId == -1 {
		return fmt.Errorf("Failed to find a fitted NUMA")
	}

	zone, err := getNUMAZoneById(fittedNUMAId, ntr.Zones)
	if err != nil {
		return err
	}

	for _, container := range pod.Spec.Containers {
		for resourceName, reqQuan := range container.Resources.Requests {
			for _, zoneRes := range zone.Resources {
				if zoneRes.Name == string(resourceName) {
					resQuantity, err := resource.ParseQuantity(zoneRes.Allocatable.String())

					if err != nil {
						klog.Errorf("Failed to parse %s", zoneRes.Allocatable.String())
						return err
					}

					resQuantity.Sub(reqQuan)
					zoneRes.Allocatable = intstr.Parse(resQuantity.String())
				}
			}
		}
	}
	return nil
}

func isNumaFit(pod *v1.Pod, numa *NUMANode) bool {
	for _, container := range pod.Spec.Containers {
		for resourceName, reqQuan := range container.Resources.Requests {
			if allocQuan, ok := numa.Resources[resourceName]; ok && allocQuan.Cmp(reqQuan) >= 0 {
				continue
			} else {
				return false
			}
		}
	}
	return true
}

func getNUMAZoneById(id int, zones topologyv1alpha1.ZoneList) (zone *topologyv1alpha1.Zone, err error) {
	name := fmt.Sprintf("node-%d", id)
	for _, zone := range zones {
		if zone.Name == name {
			return &zone, nil
		}
	}
	return nil, fmt.Errorf("zone with name %v was not found", name)
}

func TestScorePlugin(t *testing.T) {
	nodeTopologies := nodeTopologyMap{}
	nodeTopologies["default/node1"] = topologyv1alpha1.NodeResourceTopology{
		ObjectMeta:       metav1.ObjectMeta{Name: "node1"},
		TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodeContainerLevel)},
		Zones: topologyv1alpha1.ZoneList{
			topologyv1alpha1.Zone{
				Name: "node-0",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					topologyv1alpha1.ResourceInfo{
						Name:        "cpu",
						Capacity:    intstr.Parse("3"),
						Allocatable: intstr.Parse("3"),
					},
				},
			},
			topologyv1alpha1.Zone{
				Name: "node-1",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					topologyv1alpha1.ResourceInfo{
						Name:        "cpu",
						Capacity:    intstr.Parse("4"),
						Allocatable: intstr.Parse("4"),
					},
				},
			},
		},
	}

	nodeTopologies["default/node2"] = topologyv1alpha1.NodeResourceTopology{
		ObjectMeta:       metav1.ObjectMeta{Name: "node2"},
		TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodeContainerLevel)},
		Zones: topologyv1alpha1.ZoneList{
			topologyv1alpha1.Zone{
				Name: "node-0",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					topologyv1alpha1.ResourceInfo{
						Name:        "cpu",
						Capacity:    intstr.Parse("2"),
						Allocatable: intstr.Parse("2"),
					},
				},
			}, topologyv1alpha1.Zone{
				Name: "node-1",
				Type: "Node",
				Resources: topologyv1alpha1.ResourceInfoList{
					topologyv1alpha1.ResourceInfo{
						Name:        "cpu",
						Capacity:    intstr.Parse("2"),
						Allocatable: intstr.Parse("2"),
					},
				},
			},
		},
	}

	nodesMap := make(map[string]*v1.Node)
	node1Resources := makeResourceListFromZones(nodeTopologies["default/node1"].Zones)
	nodesMap["node1"] = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1.NodeStatus{
			Capacity:    node1Resources,
			Allocatable: node1Resources,
		},
	}

	node2Resources := makeResourceListFromZones(nodeTopologies["default/node2"].Zones)
	nodesMap["node2"] = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1.NodeStatus{
			Capacity:    node2Resources,
			Allocatable: node2Resources,
		},
	}

	type podRequests struct {
		pod        *v1.Pod
		name       string
		wantStatus *framework.Status
	}
	pRequests := []podRequests{
		{
			pod: makePodByResourceList(&v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI)}),
			name:       "Request #1",
			wantStatus: nil,
		},
		{
			pod: makePodByResourceList(&v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(3, resource.DecimalSI)}),
			name:       "Request #2",
			wantStatus: nil,
		},
		{
			pod: makePodByResourceList(&v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(3, resource.DecimalSI)}),
			name:       "Request #3",
			wantStatus: nil,
		},
	}

	// Each testScenario will describe a set pod requests arrived sequentially to the scoring plugin.
	type testScenario struct {
		name        string
		electedNode []string
		requests    []podRequests
	}
	// We would expect the Scoring plugin to elect the node (by giving him the highest score)
	// as described in the scenario's electedNode member,
	// where each pod request corespond to an elected node
	// For example:
	// electedNode = ["node2", "node1", "node2", "node3"]
	// Means that for the i'th pod request, we expect electedNode[i]
	// to be the node which selected by the Scoring function.
	// So for request number 0, electedNode[0] will be selected by the scoring function,
	// which is node2. For request number 3 electedNode[2] will be selected, which is node3.
	// If that's not the case the test will fail.

	tests := []testScenario{
		{
			name:        "Scenario #1",
			electedNode: []string{"node2", "node1", "node1"},
			requests:    pRequests,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			NewResourceAllocationScorer := &resourceAllocationScorer{
				scoreStrategy: getScoreStrategy("leastAllocatable"),
				data: commonPluginsData{
					lister:     listerv1alpha1.NewNodeResourceTopologyLister(mockIndexer{nodeTopologies: nodeTopologies}),
					namespaces: []string{metav1.NamespaceDefault},
				},
			}
			for id, req := range test.requests {
				nodeToScoreMap := make(map[string]int64, len(nodesMap))

				for _, node := range nodesMap {
					score, gotStatus := NewResourceAllocationScorer.Score(
						context.Background(),
						framework.NewCycleState(),
						req.pod,
						node.ObjectMeta.Name)

					fmt.Printf("test.Name: %v; request: %v; node: %v; score: %v; status: %v\n",
						test.name,
						req.name,
						node.ObjectMeta.Name,
						score,
						gotStatus)

					if !reflect.DeepEqual(gotStatus, req.wantStatus) {
						t.Errorf("status does not match: %v, want: %v\n", gotStatus, req.wantStatus)
					}
					nodeToScoreMap[node.ObjectMeta.Name] = score
				}
				expectedNodeName := test.electedNode[id]
				actualNodeName := findMaxScoreNode(nodeToScoreMap)

				fmt.Printf("test.Name: %v; elected node for %v is: %v \n", test.name, req.name, actualNodeName)
				if expectedNodeName != actualNodeName {
					t.Errorf("Failed to select the desired node: expected: %v, actual: %v", expectedNodeName, actualNodeName)
				}
				// since a request has been fulfiled by the elected node,
				// we should recalculate the amount of it's allocatable resources
				recalcResources(req.pod, nodesMap[actualNodeName])
				err := updateNrt(req.pod, findNodeTopology(actualNodeName, &NewResourceAllocationScorer.data))
				if err != nil {
					t.Errorf("Failed to update node topology for %v. error:%v", actualNodeName, err)
				}
			}
		})
	}
}
