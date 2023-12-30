// Copyright The HTNN Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pkg

import (
	"encoding/json"
	"strings"

	istiov1a3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istiov1b1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	mosniov1 "mosn.io/moe/controller/api/v1"
)

func MapToObj(in map[string]interface{}) client.Object {
	var out client.Object
	data, _ := json.Marshal(in)
	group := in["apiVersion"].(string)
	if strings.HasPrefix(group, "networking.istio.io") {
		switch in["kind"] {
		case "VirtualService":
			out = &istiov1b1.VirtualService{}
		case "Gateway":
			out = &istiov1b1.Gateway{}
		case "EnvoyFilter":
			out = &istiov1a3.EnvoyFilter{}
		}
	} else if strings.HasPrefix(group, "gateway.networking.k8s.io") {
		switch in["kind"] {
		case "HTTPRoute":
			out = &gwapiv1.HTTPRoute{}
		case "Gateway":
			out = &gwapiv1.Gateway{}
		}
	} else if strings.HasPrefix(group, "mosn.io") {
		switch in["kind"] {
		case "HTTPFilterPolicy":
			out = &mosniov1.HTTPFilterPolicy{}
		}
	}
	if out == nil {
		panic("unknown crd")
	}
	json.Unmarshal(data, out)
	return out
}