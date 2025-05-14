// Copyright 2020-2025 Politecnico di Torino
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

package bastionsshtracker

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var SSHConnections = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "bastion_ssh_connections",
		Help: "SSH connections detected from bastion to targets",
	},
	[]string{"destination_ip", "destination_port"},
)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(SSHConnections)
}
