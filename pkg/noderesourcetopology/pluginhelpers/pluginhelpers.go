package pluginhelpers

import (
	"context"
	"fmt"
	"sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology"
	"sync"

	topologyv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha1"
	topoclientset "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/clientset/versioned"
	topologyinformers "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/informers/externalversions"
	listerv1alpha1 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/generated/listers/topology/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// topologyListerInstance is a Singleton object
// Should not be accessed directly.
// Should be accessed via getNodeTopologyInformer function
var (
	topologyListerInstance *listerv1alpha1.NodeResourceTopologyLister
	once                   sync.Once
)

func FindNodeTopology(nodeName string, nodeResTopoPlugin *noderesourcetopology.NodeResTopoPlugin) *topologyv1alpha1.NodeResourceTopology {
	klog.V(5).Infof("Namespaces: %s", nodeResTopoPlugin.Namespaces)
	for _, namespace := range nodeResTopoPlugin.Namespaces {
		klog.V(5).Infof("data.lister: %v", nodeResTopoPlugin.Lister)
		// NodeTopology couldn't be placed in several Namespaces simultaneously
		lister := nodeResTopoPlugin.Lister
		nodeTopology, err := (*lister).NodeResourceTopologies(namespace).Get(nodeName)
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

func InitNodeTopologyInformer(masterOverride, kubeConfigPath *string) (*listerv1alpha1.NodeResourceTopologyLister, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(*masterOverride, *kubeConfigPath)
	if err != nil {
		klog.Errorf("Cannot create kubeconfig based on: %s, %s, %v", *masterOverride, *kubeConfigPath, err)
		return nil, err
	}

	topoClient, err := topoclientset.NewForConfig(kubeConfig)
	if err != nil {
		klog.Errorf("Cannot create clientset for NodeTopologyResource: %s, %s", kubeConfig, err)
		return nil, err
	}

	topologyInformerFactory := topologyinformers.NewSharedInformerFactory(topoClient, 0)
	nodeTopologyInformer := topologyInformerFactory.Topology().V1alpha1().NodeResourceTopologies()
	nodeResourceTopologyLister := nodeTopologyInformer.Lister()

	klog.V(5).Infof("start nodeTopologyInformer")
	ctx := context.Background()
	topologyInformerFactory.Start(ctx.Done())
	topologyInformerFactory.WaitForCacheSync(ctx.Done())

	return &nodeResourceTopologyLister, nil
}

// GetNodeTopologyLister will init v1alpha1.NodeResourceTopologyInformer once and return it.
// if v1alpha1.NodeResourceTopologyInformer already initialized, the same instance will be return
func GetNodeTopologyLister(masterOverride, kubeConfigPath *string) (*listerv1alpha1.NodeResourceTopologyLister, error) {
	var err error

	once.Do(func() {
		topologyListerInstance, err = InitNodeTopologyInformer(masterOverride, kubeConfigPath)
	})

	return topologyListerInstance, err
}

func CreateNUMANodeList(zones topologyv1alpha1.ZoneList) noderesourcetopology.NUMANodeList {
	nodes := make(noderesourcetopology.NUMANodeList, 0)
	for _, zone := range zones {
		if zone.Type == "Node" {
			var numaID int
			_, err := fmt.Sscanf(zone.Name, "node-%d", &numaID)
			if err != nil {
				klog.Errorf("Invalid format: %v", zone.Name)
				continue
			}
			if numaID > 63 || numaID < 0 {
				klog.Errorf("Invalid NUMA id range: %v", numaID)
				continue
			}
			resources := extractResources(zone)
			nodes = append(nodes, noderesourcetopology.NUMANode{NUMAID: numaID, Resources: resources})
		}
	}
	return nodes
}

func MakePodByResourceList(resources *v1.ResourceList) *v1.Pod {
	return &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: *resources,
						Limits:   *resources,
					},
				},
			},
		},
	}
}

func MakeResourceListFromZones(zones topologyv1alpha1.ZoneList) v1.ResourceList {
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

func extractResources(zone topologyv1alpha1.Zone) v1.ResourceList {
	res := make(v1.ResourceList)
	for _, resInfo := range zone.Resources {
		quantity, err := resource.ParseQuantity(resInfo.Allocatable.String())
		klog.V(5).Infof("extractResources: resInfo.FilterPluginName %v, resInfo quantity %d", resInfo.Name, quantity.AsDec())
		if err != nil {
			klog.Errorf("Failed to parse %s", resInfo.Allocatable.String())
			continue
		}
		res[v1.ResourceName(resInfo.Name)] = quantity
	}
	return res
}