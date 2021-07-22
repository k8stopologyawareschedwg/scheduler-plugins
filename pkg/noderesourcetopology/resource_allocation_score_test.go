package noderesourcetopology

import (
	"context"
	"github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	"reflect"
	"testing"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	faketopologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned/fake"
	topologyinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	framework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)
type nodeToScoreMap map[string]int64

func TestNodeResourceScorePlugin(t *testing.T) {
	nodeTopologies := make([]*topologyv1alpha1.NodeResourceTopology, 3)
	nodesMap := make(map[string]*v1.Node)
	var lister v1alpha1.NodeResourceTopologyLister

	initTest := func() {
		// noderesourcetopology objects
		nodeTopologies[0] = &topologyv1alpha1.NodeResourceTopology{
			ObjectMeta:       metav1.ObjectMeta{Name: "Node1", Namespace: "default"},
			TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodeContainerLevel)},
			Zones: topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "4", "4"),
						makeTopologyResInfo(memory, "500Mi", "500Mi"),
						},
					},
				topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "4", "4"),
						makeTopologyResInfo(memory, "500Mi", "500Mi"),
					},
				},
			},
		}

		nodeTopologies[1] = &topologyv1alpha1.NodeResourceTopology{
			ObjectMeta:       metav1.ObjectMeta{Name: "Node2", Namespace: "default"},
			TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodeContainerLevel)},
			Zones: topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "2", "2"),
						makeTopologyResInfo(memory, "50Mi", "50Mi"),
					},
				}, topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "2", "2"),
						makeTopologyResInfo(memory, "50Mi", "50Mi"),
					},
				},
			},
		}
		nodeTopologies[2] = &topologyv1alpha1.NodeResourceTopology{
			ObjectMeta:       metav1.ObjectMeta{Name: "Node3", Namespace: "default"},
			TopologyPolicies: []string{string(topologyv1alpha1.SingleNUMANodeContainerLevel)},
			Zones: topologyv1alpha1.ZoneList{
				topologyv1alpha1.Zone{
					Name: "node-0",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "6", "6"),
						makeTopologyResInfo(memory, "60Mi", "60Mi"),
					},
				}, topologyv1alpha1.Zone{
					Name: "node-1",
					Type: "Node",
					Resources: topologyv1alpha1.ResourceInfoList{
						makeTopologyResInfo(cpu, "6", "6"),
						makeTopologyResInfo(memory, "60Mi", "60Mi"),
					},
				},
			},
		}
		// init node objects
		for _, nrt := range nodeTopologies {
			res := makeResourceListFromZones(nrt.Zones)
			nodesMap[nrt.Name] = &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: nrt.Name},
				Status: v1.NodeStatus{
					Capacity:    res,
					Allocatable: res,
				},
			}
		}

		// init topology lister
		fakeClient := faketopologyv1alpha1.NewSimpleClientset()
		fakeInformer := topologyinformers.NewSharedInformerFactory(fakeClient, 0).Topology().V1alpha1().NodeResourceTopologies()
		for _, obj := range nodeTopologies {
			fakeInformer.Informer().GetStore().Add(obj)
		}
		lister = fakeInformer.Lister()
	}

	type podRequests struct {
		pod        *v1.Pod
		name       string
		wantStatus *framework.Status
	}
	pRequests := []podRequests{
		{
			pod: makePodByResourceList(&v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
				v1.ResourceMemory: *resource.NewQuantity(20 * 1024 * 1024, resource.DecimalSI)}),
			name:       "Pod1",
			wantStatus: nil,
		},
	}

	// Each testScenario will describe a set pod requests arrived sequentially to the scoring plugin.
	type testScenario struct {
	name         string
	wantedRes    nodeToScoreMap
	requests     []podRequests
	strategyName string
}

	tests := []testScenario{
		{
			// On 0-MaxNodeScore scale
			// Node2 resource fractions:
			// CPU Fraction: 2 / 2 = 100%
			// Memory Fraction: 20M / 50M = 40%
			// Node2 score:(100 + 40) / 2 = 70
			name:         "least-allocatable strategy",
			wantedRes:    nodeToScoreMap{"Node2": 70},
			requests:     pRequests,
			strategyName: string(LeastAllocatable),
		},
		{
			// On 0-MaxNodeScore scale
			// Node3 resource fractions:
			// CPU Fraction: 2 / 6 = 33%
			// Memory Fraction: 20M / 60M = 33%
			// Node3 score: MaxNodeScore - (0.33 - 0.33) = MaxNodeScore
			name:         "balanced-allocation strategy",
			wantedRes:    nodeToScoreMap{"Node3": 100},
			requests:     pRequests,
			strategyName: string(BalancedAllocation),
		},
		{
			// On 0-MaxNodeScore scale
			// Node1 resource fractions:
			// CPU Fraction: 2 / 4 = 50%
			// Memory Fraction: 20M / 500M = 4%
			// Node1 score: ((100 - 50) + (100 - 4)) / 2 = 73
			name:         "most-allocatable strategy",
			wantedRes:    nodeToScoreMap{"Node1": 73},
			requests:     pRequests,
			strategyName: string(MostAllocatable),
		},
	}

	for _, test := range tests {
		initTest()
		t.Run(test.name, func(t *testing.T) {
			NewResourceAllocationScorer := &resourceAllocationScorer{
				scoreStrategy: getScoreStrategy(test.strategyName),
				NodeResTopoPlugin: NodeResTopoPlugin{
					Lister:     &lister  ,
					Namespaces: []string{metav1.NamespaceDefault},
				},
			}
			for _, req := range test.requests {
				nodeToScore := make(nodeToScoreMap, len(nodesMap))
				for _, node := range nodesMap {
					score, gotStatus := NewResourceAllocationScorer.Score(
						context.Background(),
						framework.NewCycleState(),
						req.pod,
						node.ObjectMeta.Name)

					t.Logf("%v; %v; %v; score: %v; status: %v\n",
						test.name,
						req.name,
						node.ObjectMeta.Name,
						score,
						gotStatus)

					if !reflect.DeepEqual(gotStatus, req.wantStatus) {
						t.Errorf("status does not match: %v, want: %v\n", gotStatus, req.wantStatus)
					}
					nodeToScore[node.ObjectMeta.Name] = score
				}
				gotNode := findMaxScoreNode(nodeToScore)
				gotScore:= nodeToScore[gotNode]
				t.Logf("%q: got node %q with score %d\n", test.name, gotNode, gotScore)
				for wantNode, wantScore := range test.wantedRes {
					if wantNode != gotNode {
						t.Errorf("failed to select the desired node: wanted: %q, got: %q", wantNode, gotNode)
					}

					if wantScore != gotScore {
						t.Errorf("wrong score for node %q: wanted: %q, got: %d", gotNode, wantScore, gotScore)
					}
				}
			}
		})
	}
}

// return the name of the node with the highest score
func findMaxScoreNode(nodeToScore nodeToScoreMap) string {
	max := int64(0)
	electedNode := ""

	for nodeName, score := range nodeToScore {
		if max <= score {
			max = score
			electedNode = nodeName
		}
	}
	return electedNode
}