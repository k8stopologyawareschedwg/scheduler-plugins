package noderesourcetopology

import (
	"context"
	"fmt"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topoclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	topologyinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	v1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// no no mock only indexer
type mockIndexer struct {
	nodeTopologies nodeTopologyMap
}

func (m mockIndexer) ByIndex(indexName, indexKey string) ([]interface{}, error)      { return nil, nil }
func (m mockIndexer) AddIndexers(cache.Indexers) error                               { return nil }
func (m mockIndexer) GetIndexers() cache.Indexers                                    { return nil }
func (m mockIndexer) ListIndexFuncValues(indexName string) []string                  { return nil }
func (m mockIndexer) Index(indexName string, obj interface{}) ([]interface{}, error) { return nil, nil }
func (m mockIndexer) IndexKeys(indexName, indexedValue string) ([]string, error)     { return nil, nil }

func (m mockIndexer) Add(interface{}) error               { return nil }
func (m mockIndexer) Update(obj interface{}) error        { return nil }
func (m mockIndexer) Delete(obj interface{}) error        { return nil }
func (m mockIndexer) List() []interface{}                 { return nil }
func (m mockIndexer) ListKeys() []string                  { return nil }
func (m mockIndexer) Replace([]interface{}, string) error { return nil }
func (m mockIndexer) Resync() error                       { return nil }
func (m mockIndexer) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}

func (m mockIndexer) GetByKey(key string) (item interface{}, exists bool, err error) {
	if v, ok := m.nodeTopologies[key]; ok {
		return &v, ok, nil
	}
	return nil, false, fmt.Errorf("Node topology is not found: %v", key)
}

func findNodeTopology(nodeName string, data *commonPluginsData) *topologyv1alpha1.NodeResourceTopology {
	klog.V(5).Infof("namespaces: %s", data.namespaces)
	for _, namespace := range data.namespaces {
		// NodeTopology couldn't be placed in several namespaces simultaneously
		nodeTopology, err := data.lister.NodeResourceTopologies(namespace).Get(nodeName)
		if err != nil {
			klog.V(5).Infof("Cannot get NodeTopologies from NodeResourceTopologyNamespaceLister: %v", err)
			continue
		}
		if nodeTopology != nil {
			return nodeTopology
		}
	}
	return nil
}

func initNodeTopologyInformer(masterOverride, kubeConfigPath string) (v1alpha1.NodeResourceTopologyInformer, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(masterOverride, kubeConfigPath)
	if err != nil {
		klog.Errorf("Cannot create kubeconfig based on: %s, %s, %v", masterOverride, kubeConfigPath, err)
		return nil, err
	}

	topoClient, err := topoclientset.NewForConfig(kubeConfig)
	if err != nil {
		klog.Errorf("Cannot create clientset for NodeTopologyResource: %s, %s", kubeConfig, err)
		return nil, err
	}
	topologyInformerFactory := topologyinformers.NewSharedInformerFactory(topoClient, 0)
	klog.V(5).Infof("start nodeTopologyInformer")
	ctx := context.Background()
	topologyInformerFactory.Start(ctx.Done())
	topologyInformerFactory.WaitForCacheSync(ctx.Done())
	return topologyInformerFactory.Topology().V1alpha1().NodeResourceTopologies(), nil
}

func extractResources(zone topologyv1alpha1.Zone) v1.ResourceList {
	res := make(v1.ResourceList)
	for _, resInfo := range zone.Resources {
		quantity, err := resource.ParseQuantity(resInfo.Allocatable.String())
		if err != nil {
			klog.Errorf("Failed to parse %s", resInfo.Allocatable.String())
			continue
		}
		res[v1.ResourceName(resInfo.Name)] = quantity
	}
	return res
}

func createNUMANodeList(zones topologyv1alpha1.ZoneList) NUMANodeList {
	nodes := make(NUMANodeList, 0)
	for _, zone := range zones {
		if zone.Type == "Node" {
			var numaID int
			_, err := fmt.Sscanf(zone.Name, "node-%d", &numaID)
			if err != nil {
				klog.Errorf("Invalid format: %v", zone.Name)
				continue
			}
			resources := extractResources(zone)
			nodes = append(nodes, NUMANode{NUMAID: numaID, Resources: resources})
		}
	}
	return nodes
}

func makePodByResourceList(resources *v1.ResourceList) *v1.Pod {
	return &v1.Pod{Spec: v1.PodSpec{Containers: []v1.Container{{
		Resources: v1.ResourceRequirements{
			Requests: *resources,
			Limits:   *resources,
		},
	}},
	}}
}

func makeResourceListFromZones(zones topologyv1alpha1.ZoneList) v1.ResourceList {
	result := make(v1.ResourceList)
	for _, zone := range zones {
		for _, resInfo := range zone.Resources {
			resQuantity, err := resource.ParseQuantity(resInfo.Allocatable.String())
			if err != nil {
				klog.Errorf("Failed to parse %s", resInfo.Allocatable.String())
				continue
			}
			if quantity, ok := result[v1.ResourceName(resInfo.Name)]; ok {
				resQuantity.Add(quantity)
			}
			result[v1.ResourceName(resInfo.Name)] = resQuantity
		}
	}
	return result
}
