// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by defaulter-gen. DO NOT EDIT.

package v1beta1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// RegisterDefaults adds defaulters functions to the given scheme.
// Public to allow building arbitrary schemes.
// All generated defaulters are covering - they call all nested defaulters.
func RegisterDefaults(scheme *runtime.Scheme) error {
	scheme.AddTypeDefaultingFunc(&CapacitySchedulingArgs{}, func(obj interface{}) { SetObjectDefaultsCapacitySchedulingArgs(obj.(*CapacitySchedulingArgs)) })
	scheme.AddTypeDefaultingFunc(&CoschedulingArgs{}, func(obj interface{}) { SetObjectDefaultsCoschedulingArgs(obj.(*CoschedulingArgs)) })
	scheme.AddTypeDefaultingFunc(&NodeResourcesAllocatableArgs{}, func(obj interface{}) {
		SetObjectDefaultsNodeResourcesAllocatableArgs(obj.(*NodeResourcesAllocatableArgs))
	})
	scheme.AddTypeDefaultingFunc(&TargetLoadPackingArgs{}, func(obj interface{}) {
		SetObjectDefaultsTargetLoadPackingArgs(obj.(*TargetLoadPackingArgs))
	})
	scheme.AddTypeDefaultingFunc(&NodeResourceTopologyMatchArgs{}, func(obj interface{}) {
		SetDefaultsNodeResourceTopologyMatchArgs(obj.(*NodeResourceTopologyMatchArgs))
	})
	return nil
}

func SetObjectDefaultsCapacitySchedulingArgs(in *CapacitySchedulingArgs) {
	SetDefaultsCapacitySchedulingArgs(in)
}

func SetObjectDefaultsCoschedulingArgs(in *CoschedulingArgs) {
	SetDefaultsCoschedulingArgs(in)
}

func SetObjectDefaultsNodeResourcesAllocatableArgs(in *NodeResourcesAllocatableArgs) {
	SetDefaultsNodeResourcesAllocatableArgs(in)
}

func SetObjectDefaultsTargetLoadPackingArgs(in *TargetLoadPackingArgs) {
	SetDefaultTargetLoadPackingArgs(in)
}
