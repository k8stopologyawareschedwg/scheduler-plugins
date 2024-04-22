/*
Copyright 2021 The Kubernetes Authors.

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

package logging

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

// before to replace with FromContext(), at least in filter and score,
// we would need a way to inject a logger instance (preferably a
// per-plugin logger instance) when we create the Scheduler
// (with app.NewSchedulerCommand)

// well-known structured log keys
const (
	KeyLogID  string = "logID"
	KeyPod    string = "pod"
	KeyPodUID string = "podUID"
	KeyNode   string = "node"
	KeyFlow   string = "flow"
)

const (
	FlowBegin string = "begin"
	FlowEnd   string = "end"
)

const (
	SubsystemForeignPods string = "foreignpods"
	SubsystemNRTCache    string = "nrtcache"
)

const (
	FlowCacheSync string = "cachesync"
	FlowFilter    string = "filter"
	FlowPostBind  string = "postbind"
	FlowReserve   string = "reserve"
	FlowScore     string = "score"
	FlowUnreserve string = "unreserve"
)

// we would like to inject loggers in the scheduler-framework-managed
// context. Best would be one per-stage (e.g. filter, score...) but
// one per-plugin (e.g. NodeResourceFit, NodeResourceTopology...) works
// as well. Till this is possible, we reserve the option to optionally
// override the context-provided logger. This is why we also have
// out own FromContext instead of a smaller helper.
var logHandler logr.Logger
var logOverridden bool

func init() {
	logHandler = klog.Background()
	logOverridden = false
}

// Setup must be called once before the plugin code is executed
func Setup(lh logr.Logger) {
	logHandler = lh
	logOverridden = true
}

func FromContext(ctx context.Context) logr.Logger {
	lh := klog.FromContext(ctx)
	if logOverridden {
		lh = logHandler
	}
	return lh
}

func FromContextWithValues(ctx context.Context, pod *corev1.Pod, nodeName, flowName string) logr.Logger {
	lh := FromContext(ctx)
	if logOverridden {
		// consistency with what scheduler framework does.
		// we intentionally fully trust the provided logger about extra values,
		// typically node(Name) being processed and flow name (using WithName).
		// this is because we prefer to avoid awkward duplication of name and flow,
		// klog doesn't give us a facility to deduplicate these values.
		if lh.V(4).Enabled() {
			lh = lh.WithName(flowName).WithValues(KeyNode, nodeName)
		}
	}
	return withPodValues(lh, pod)
}

func withPodValues(lh logr.Logger, pod *corev1.Pod) logr.Logger {
	// these are infos we always want, if available, even regardless the V level
	if pod == nil {
		return lh
	}
	return lh.WithValues(KeyPod, PodLogID(pod), KeyPodUID, pod.GetUID())
}

func PodLogID(pod *corev1.Pod) string {
	if pod == nil {
		return "<nil>"
	}
	if pod.Namespace == "" {
		return pod.Name
	}
	return pod.Namespace + "/" + pod.Name
}

func TimeLogID() string {
	return fmt.Sprintf("uts/%v", time.Now().UnixMilli())
}
